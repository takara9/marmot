package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"

	cf "github.com/takara9/marmot/pkg/config"
)

type Db struct {
	EndpointUrl string
	Cli         *clientv3.Client
	Ctx         context.Context
	Cancel      context.CancelFunc
}

// etcdへ接続
func NewEtcdEp(url string) (Db, error) {
	var con Db
	var err error
	con.EndpointUrl = url
	con.Cli, err = clientv3.New(clientv3.Config{
		Endpoints:   []string{url},
		DialTimeout: 2 * time.Second,
	})
	con.Ctx, con.Cancel = context.WithTimeout(context.Background(), 2*time.Second)

	return con, err
}

// 前方一致のサーチ
func (d *Db) GetEtcdByPrefix(key string) (*clientv3.GetResponse, error) {
	resp, err := d.Cli.Get(d.Ctx, key, clientv3.WithPrefix())

	return resp, err
}

// Keyに一致したHVデータの取り出し
func (d *Db) GetHvByKey(key string) (Hypervisor, error) {
	var hv Hypervisor
	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return hv, err
	} else if resp.Count == 0 {
		return hv, errors.New("not found")
	}
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &hv)

	return hv, err
}

// Keyに一致したVMデータの取り出し
func (d *Db) GetVmByKey(key string) (VirtualMachine, error) {
	var vm VirtualMachine
	if len(key) == 0 {
		return vm, errors.New("not found")
	}

	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return vm, err
	}
	if resp.Count == 0 {
		return vm, errors.New("not found")
	}
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &vm)

	return vm, err
}

// Keyに一致したOSイメージテンプレートを返す
func (d *Db) GetOsImgTempByKey(osv string) (string, string, error) {
	key := fmt.Sprintf("OSI_%v", osv)
	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return "", "", err
	}

	if resp.Count == 0 {
		return "", "", errors.New("NotFound")
	}

	var oit OsImageTemplate
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &oit)
	if err != nil {
		return "", "", err
	}

	return oit.VolumeGroup, oit.LogicaVol, nil
}

func (d *Db) GetEtcdByKey(path string) (DNSEntry, error) {
	var entry DNSEntry
	resp, err := d.Cli.Get(d.Ctx, path)
	if err != nil {
		return entry, err
	}

	if resp.Count == 0 {
		return entry, errors.New("not found")
	}

	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &entry)
	if err != nil {
		return entry, err
	}

	return entry, nil
}

// 削除 キーに一致したデータ
func (d *Db) DelByKey(key string) error {
	_, err := d.Cli.Delete(d.Ctx, key)

	return err
}

// ハイパーバイザーのデータを取得
func (d *Db) GetHvsStatus(hvs *[]Hypervisor) error {
	resp, err := d.GetEtcdByPrefix("hv")
	if err != nil {
		return err
	}
	for _, ev := range resp.Kvs {
		var hv Hypervisor
		err = json.Unmarshal([]byte(ev.Value), &hv)
		if err != nil {
			return err
		}
		*hvs = append(*hvs, hv)
	}

	return nil
}

// 仮想マシンのデータを取得
func (d *Db) GetVmsStatus(vms *[]VirtualMachine) error {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return err
	}
	for _, ev := range resp.Kvs {
		var vm VirtualMachine // ここに宣言することで、ループ毎に初期化される
		err = json.Unmarshal(ev.Value, &vm)
		if err != nil {
			return err
		}
		*vms = append(*vms, vm)
	}

	return nil
}

// etcdへ保存
func (d *Db) PutDataEtcd(k string, v interface{}) error {
	byteJSON, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = d.Cli.Put(d.Ctx, k, string(byteJSON))

	return err
}

// シリアル番号
func (d *Db) CreateSeq(key string, start uint64, step uint64) error {
	etcd_key := fmt.Sprintf("SEQNO_%v", key)
	var seq VmSerial
	seq.Serial = start
	seq.Start = start
	seq.Step = step
	seq.Key = key
	err := d.PutDataEtcd(etcd_key, seq)

	return err
}

// シリアル番号の取得
func (d *Db) GetSeq(key string) (uint64, error) {
	var seq VmSerial
	etcdKey := fmt.Sprintf("SEQNO_%v", key)
	resp, err := d.Cli.Get(d.Ctx, etcdKey)
	if err != nil {
		return 0, err
	}

	err = json.Unmarshal(resp.Kvs[0].Value, &seq)
	if err != nil {
		return 0, err
	}

	seqno := seq.Serial
	seq.Serial = seq.Serial + seq.Step

	err = d.PutDataEtcd(etcdKey, seq)
	if err != nil {
		return 0, err
	}

	return seqno, nil
}

func (d *Db) DelSeq(key string) error {
	etcdKey := fmt.Sprintf("SEQNO_%v", key)
	err := d.DelByKey(etcdKey)

	return err
}

// 空きハイパーバイザーに仮想マシンを割り当てる
// 割り当てたハイパーバイザーのリソースを減らす
// 仮想マシンのデータをセットする
// 仮想マシンの状態をプロビジョニング中にする
func (d *Db) AssignHvforVm(vm VirtualMachine) (string, string, uuid.UUID, error) {
	var txId = uuid.New()
	//トランザクション開始、他更新ロック
	// 仮想マシンをデータベースに登録、状態は「データ登録中」
	var hvs []Hypervisor
	err := d.GetHvsStatus(&hvs) // HVのステータス取得
	if err != nil {
		return "", "", txId, err
	}

	// フリーのCPU数の降順に並べ替える
	sort.Slice(hvs, func(i, j int) bool { return hvs[i].FreeCpu > hvs[j].FreeCpu })

	// リソースに空きのあるハイパーバイザーを探す
	var assigned = false
	var hv Hypervisor
	for _, hv = range hvs {

		// 停止中のHVの割り当てない
		//fmt.Println("============= hv status ", hv.Status)
		if hv.Status != 2 {
			continue
		}

		if hv.FreeCpu >= vm.Cpu {
			if hv.FreeMemory >= vm.Memory {

				hv.FreeMemory = hv.FreeMemory - vm.Memory
				hv.FreeCpu = hv.FreeCpu - vm.Cpu
				// ストレージの容量管理は未実装

				vm.Status = 0           // 登録中
				vm.HvNode = hv.Nodename // ハイパーバイザーを決定
				assigned = true
				break
			}
		}
	}
	// リソースに空きが無い場合はエラーを返す
	if !assigned {
		err := errors.New("could't assign VM due to doesn't have enough a resouce on HV")
		return "", "", txId, err
	}
	// ハイパーバイザーのリソース削減保存
	err = d.PutDataEtcd(hv.Key, hv)
	if err != nil {
		return "", "", txId, err
	}
	// VM名登録 シリアル番号取得
	seqno, err := d.GetSeq("VM")
	if err != nil {
		return "", "", txId, err
	}

	vm.Key = fmt.Sprintf("vm_%s_%04d", vm.Name, seqno)
	//vm.NameはOSホスト名なので受けたものを利用
	vm.Uuid = txId
	vm.Ctime = time.Now()
	vm.Stime = time.Now()
	//vm.Status = 1  // 状態プロビ中
	err = d.PutDataEtcd(vm.Key, vm) // 仮想マシンのデータ登録

	return vm.HvNode, vm.Key, vm.Uuid, err
}

// VMの終了とリソースの開放
func (d *Db) RemoveVmFromHV(vmKey string) error {
	// トランザクションであるべき？
	// VMをキーで取得して、ハイパーバイザーを取得
	vm, err := d.GetVmByKey(vmKey)
	if err != nil {
		return err
	}
	hv, err := d.GetHvByKey(vm.HvNode)
	if err != nil {
		return err
	}
	// HVからリソースを削除
	hv.FreeCpu = hv.FreeCpu + vm.Cpu
	hv.FreeMemory = hv.FreeMemory + vm.Memory
	err = d.PutDataEtcd(vm.HvNode, &hv)
	if err != nil {
		return err
	}
	// VMを削除
	err = d.DelByKey(vm.Key)
	if err != nil {
		return err
	}

	return nil
}

// パブリックIPアドレスが一致するインスタンスを探す
func (d *Db) FindByPublicIPaddress(ipAddress string) (bool, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return false, err
	}
	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return false, nil /// 例外的にエラーを無視
		}
		fmt.Println("===========- ipAddress=", ipAddress, "  vm.PublicIp=", vm.PublicIp)
		if ipAddress == vm.PublicIp {
			return true, nil
		}
	}

	return false, nil
}

// プライベートIPアドレスが一致するインスンスを探す
func (d *Db) FindByPrivateIPaddress(ipAddress string) (bool, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return false, err
	}
	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return false, nil /// データが存在しない時には、どうするか？
		}
		fmt.Println("===========- ipAddress=", ipAddress, "  vm.PrivateIp=", vm.PrivateIp)
		if ipAddress == vm.PrivateIp {
			return true, nil
		}
	}

	return false, nil
}

// ホスト名からVMキーを探す
func (d *Db) FindByHostname(hostname string) (string, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return "", err
	}
	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return "", err
		}
		if hostname == vm.Name {
			return vm.Key, err
		}
	}

	return "", errors.New("NotFound")
}

// ホスト名とクラスタ名でVMキーを取得する
func (d *Db) FindByHostAndClusteName(hostname string, clustername string) (string, error) {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return "", err
	}

	for _, ev := range resp.Kvs {
		var vm VirtualMachine
		err = json.Unmarshal([]byte(ev.Value), &vm)
		if err != nil {
			return "", err
		}
		if hostname == vm.Name && clustername == vm.ClusterName {
			return vm.Key, err
		}
	}

	return "", errors.New("NotFound")
}

// OSボリュームのLVをetcdへ登録
func (d *Db) UpdateOsLv(vmkey string, vg string, lv string) error {
	// ロックしたい
	vm, err := d.GetVmByKey(vmkey)
	if err != nil {
		return err
	}
	vm.OsLv = lv
	vm.OsVg = vg
	err = d.PutDataEtcd(vmkey, vm)

	return err
}

// データボリュームLVをetcdへ登録
func (d *Db) UpdateDataLv(vmkey string, idx int, vg string, lv string) error {
	// ロックしたい
	vm, err := d.GetVmByKey(vmkey)
	if err != nil {
		return err
	}
	vm.Storage[idx].Lv = lv
	vm.Storage[idx].Vg = vg
	err = d.PutDataEtcd(vmkey, vm)

	return err
}

func (d *Db) UpdateVmState(vmkey string, state int) error {
	// ロックしたい
	vm, err := d.GetVmByKey(vmkey)
	if err != nil {
		return err
	}
	vm.Status = state
	err = d.PutDataEtcd(vmkey, vm)

	return err
}

// ハイパーバイザーの設定
func (d *Db) SetHypervisor(v cf.Hypervisor_yaml) error {
	var hv Hypervisor

	hv.Nodename = v.Name
	hv.Key = v.Name // Key
	hv.IpAddr = v.IpAddr
	hv.Cpu = int(v.Cpu)
	hv.FreeCpu = int(v.Cpu)       // これで良いのか
	hv.Memory = int(v.Ram * 1024) // MB
	hv.FreeMemory = int(v.Ram * 1024)
	hv.Status = 2 // テストのため暫定

	for _, val := range v.Storage {
		var sp StoragePool
		sp.VolGroup = val.VolGroup
		sp.Type = val.Type
		hv.StgPool = append(hv.StgPool, sp)
	}

	err := d.PutDataEtcd(hv.Key, hv)
	if err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}

	return nil
}

// イメージテンプレート
func (d *Db) SetImageTemplate(v cf.Image_yaml) error {
	var osi OsImageTemplate
	osi.LogicaVol = v.LogicalVolume
	osi.VolumeGroup = v.VolumeGroup
	osi.OsVariant = v.Name
	key := fmt.Sprintf("%v_%v", "OSI", osi.OsVariant)
	err := d.PutDataEtcd(key, osi)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	return nil
}
