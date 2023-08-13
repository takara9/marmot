package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	etcd "go.etcd.io/etcd/client/v3"
)

/*
   タイムアウトを取る様に修正が必要
   テスト
   GoDoc
*/

// etcdへ接続
func Connect(url1 string) (*etcd.Client, error) {
	conn, err := etcd.New(etcd.Config{
		Endpoints:   []string{url1},
		DialTimeout: 2 * time.Second,
	})
	return conn, err
}

// 前方一致のサーチ
func GetEtcdByPrefix(con *etcd.Client, key string) (*etcd.GetResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	resp, err := con.Get(ctx, key, etcd.WithPrefix())
	cancel()
	return resp, err
}

// Keyに一致したHVデータの取り出し
func GetHvByKey(con *etcd.Client, key string) (Hypervisor, error) {
	var hv Hypervisor

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	resp, err := con.Get(ctx, key)
	cancel()

	if err != nil {
		return hv, err
	}

	// エラー処理として正しくない
	if resp.Count == 0 {
		return hv, err
	}
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &hv)

	return hv, err
}

// Keyに一致したVMデータの取り出し
func GetVmByKey(con *etcd.Client, key string) (VirtualMachine, error) {
	var vm VirtualMachine

	if len(key) == 0 {
		return vm, errors.New("NotFound")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	resp, err := con.Get(ctx, key)
	cancel()

	if err != nil {
		return vm, err
	}

	// エラー処理として正しくない
	if resp.Count == 0 {
		return vm, err
	}
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &vm)

	return vm, err
}

// Keyに一致したOSイメージテンプレートを返す
func GetOsImgTempByKey(con *etcd.Client, osv string) (string, string, error) {

	key := fmt.Sprintf("OSI_%v", osv)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	resp, err := con.Get(ctx, key)
	cancel()
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

// 削除 キーに一致したデータ
func DelByKey(con *etcd.Client, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, err := con.Delete(ctx, key)
	cancel()
	return err
}

// ハイパーバイザーのデータを取得
func GetHvsStatus(con *etcd.Client, hvs *[]Hypervisor) error {
	resp, err := GetEtcdByPrefix(con, "hv")
	if err != nil {
		return err
	}
	//var hv Hypervisor
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
func GetVmsStatus(con *etcd.Client, vms *[]VirtualMachine) error {
	resp, err := GetEtcdByPrefix(con, "vm")
	if err != nil {
		return err
	}
	//var vm VirtualMachine  ここに書くと、ループ内で初期化されないで、
	//                       上書きされるので、バグの原因になる。
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
func PutDataEtcd(con *etcd.Client, k string, v interface{}) error {
	byteJSON, err := json.Marshal(v)
	_, err = con.Put(context.TODO(), k, string(byteJSON))
	if err != nil {
		return err
	}
	return nil
}

// シリアル番号
func CreateSeq(con *etcd.Client, key string, start uint64, step uint64) error {
	etcd_key := fmt.Sprintf("SEQNO_%v", key)
	var seq VmSerial
	seq.Serial = start
	seq.Start = start
	seq.Step = step
	seq.Key = key
	err := PutDataEtcd(con, etcd_key, seq)
	return err
}

// シリアル番号の取得
func GetSeq(con *etcd.Client, key string) (uint64, error) {
	var seq VmSerial

	etcdKey := fmt.Sprintf("SEQNO_%v", key)
	resp, err := con.Get(context.TODO(), etcdKey)
	if err != nil {
		return 0, err
	}

	err = json.Unmarshal(resp.Kvs[0].Value, &seq)
	seqno := seq.Serial
	seq.Serial = seq.Serial + seq.Step

	err = PutDataEtcd(con, etcdKey, seq)
	if err != nil {
		return 0, err
	}
	return seqno, nil
}

func DelSeq(con *etcd.Client, key string) error {
	etcdKey := fmt.Sprintf("SEQNO_%v", key)
	err := DelByKey(con, etcdKey)
	return err
}

// 空きハイパーバイザーに仮想マシンを割り当てる
// 割り当てたハイパーバイザーのリソースを減らす
// 仮想マシンのデータをセットする
// 仮想マシンの状態をプロビジョニング中にする
func AssignHvforVm(con *etcd.Client, vm VirtualMachine) (string, string, uuid.UUID, error) {

	var txId = uuid.New()
	//トランザクション開始、他更新ロック
	// 仮想マシンをデータベースに登録、状態は「データ登録中」
	var hvs []Hypervisor
	err := GetHvsStatus(con, &hvs) // HVのステータス取得
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
	if assigned == false {
		err := errors.New("Could't assign VM due to doesn't have enough a resouce on HV")
		return "", "", txId, err
	}
	// ハイパーバイザーのリソース削減保存
	err = PutDataEtcd(con, hv.Key, hv)
	if err != nil {
		return "", "", txId, err
	}
	// VM名登録　シリアル番号取得
	seqno, err := GetSeq(con, "VM")
	if err != nil {
		return "", "", txId, err
	}

	vm.Key = fmt.Sprintf("vm_%s_%04d", vm.Name, seqno)
	//vm.NameはOSホスト名なので受けたものを利用
	vm.Uuid = txId
	vm.Ctime = time.Now()
	vm.Stime = time.Now()
	//vm.Status = 1  // 状態プロビ中
	err = PutDataEtcd(con, vm.Key, vm) // 仮想マシンのデータ登録
	return vm.HvNode, vm.Key, vm.Uuid, err
}

// VMの終了とリソースの開放
func RemoveVmFromHV(con *etcd.Client, vmKey string) error {

	// トランザクションであるべき？
	// VMをキーで取得して、ハイパーバイザーを取得
	vm, err := GetVmByKey(con, vmKey)
	if err != nil {
		return err
	}
	hv, err := GetHvByKey(con, vm.HvNode)
	if err != nil {
		return err
	}
	// HVからリソースを削除
	hv.FreeCpu = hv.FreeCpu + vm.Cpu
	hv.FreeMemory = hv.FreeMemory + vm.Memory
	err = PutDataEtcd(con, vm.HvNode, &hv)
	if err != nil {
		return err
	}
	// VMを削除
	err = DelByKey(con, vm.Key)
	if err != nil {
		return err
	}
	return nil
}

// ホスト名からVMキーを探す
func FindByHostname(con *etcd.Client, hostname string) (string, error) {
	resp, err := GetEtcdByPrefix(con, "vm")
	if err != nil {
		return "", err
	}
	//var vm VirtualMachine
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
func FindByHostAndClusteName(con *etcd.Client, hostname string, clustername string) (string, error) {
	resp, err := GetEtcdByPrefix(con, "vm")
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
func UpdateOsLv(con *etcd.Client, vmkey string, vg string, lv string) error {
	// ロックしたい
	vm, err := GetVmByKey(con, vmkey)
	if err != nil {
		return err
	}
	vm.OsLv = lv
	vm.OsVg = vg
	err = PutDataEtcd(con, vmkey, vm)
	return err
}

// データボリュームLVをetcdへ登録
func UpdateDataLv(con *etcd.Client, vmkey string, idx int, vg string, lv string) error {
	// ロックしたい
	vm, err := GetVmByKey(con, vmkey)
	if err != nil {
		return err
	}
	vm.Storage[idx].Lv = lv
	vm.Storage[idx].Vg = vg
	err = PutDataEtcd(con, vmkey, vm)
	return err
}

func UpdateVmState(con *etcd.Client, vmkey string, state int) error {
	// ロックしたい
	vm, err := GetVmByKey(con, vmkey)
	if err != nil {
		return err
	}
	vm.Status = state
	err = PutDataEtcd(con, vmkey, vm)
	return err
}
