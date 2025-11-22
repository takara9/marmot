package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/takara9/marmot/api"
)

// Keyに一致したVMデータの取り出し
func (d *Database) GetVmByKey(key string) (api.VirtualMachine, error) {
	var vm api.VirtualMachine
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

// 仮想マシンのデータを取得
func (d *Database) GetVmsStatus(vms *[]api.VirtualMachine) error {
	resp, err := d.GetEtcdByPrefix("vm")
	if err != nil {
		return err
	}
	for _, ev := range resp.Kvs {
		var vm api.VirtualMachine // ここに宣言することで、ループ毎に初期化される
		err = json.Unmarshal(ev.Value, &vm)
		if err != nil {
			return err
		}
		*vms = append(*vms, vm)
	}
	return nil
}

// 空きハイパーバイザーに仮想マシンを割り当てる
// 割り当てたハイパーバイザーのリソースを減らす
// 仮想マシンのデータをセットする
// 仮想マシンの状態をプロビジョニング中にする
func (d *Database) AssignHvforVm(vm api.VirtualMachine) (string, string, string, string, int32, error) {

	var txId = uuid.New()
	//トランザクション開始、他更新ロック 仮想マシンをデータベースに登録、状態は「データ登録中」
	var hvs []api.Hypervisor
	err := d.GetHypervisors(&hvs) // HVのステータス取得
	if err != nil {
		return "", "", "", txId.String(), 0, err
	}

	// フリーのCPU数の降順に並べ替える
	sort.Slice(hvs, func(i, j int) bool { return *hvs[i].FreeCpu > *hvs[j].FreeCpu })

	// リソースに空きのあるハイパーバイザーを探す
	var assigned = false
	var hv api.Hypervisor
	//var port int
	for _, hv = range hvs {

		// 停止中のHVの割り当てない
		if *hv.Status != 2 {
			continue
		}

		if *hv.FreeCpu >= *vm.Cpu {
			if *hv.FreeMemory >= *vm.Memory {
				*hv.FreeMemory = *hv.FreeMemory - *vm.Memory
				*hv.FreeCpu = *hv.FreeCpu - *vm.Cpu
				// ストレージの容量管理は未実装
				vm.Status = int32Ptr(0) // 登録中
				vm.HvNode = hv.NodeName // ハイパーバイザーを決定
				vm.HvIpAddr = hv.IpAddr
				vm.HvPort = hv.Port
				assigned = true
				break
			}
		}
	}
	// リソースに空きが無い場合はエラーを返す
	if !assigned {
		err := errors.New("could't assign VM due to doesn't have enough a resouce on HV")
		return "", "", "", txId.String(), 0, err
	}
	// ハイパーバイザーのリソース削減保存
	err = d.PutDataEtcd(*hv.Key, hv)
	if err != nil {
		return "", "", "", txId.String(), 0, err
	}
	// VM名登録　シリアル番号取得
	seqNum, err := d.GetSeq("VM")
	if err != nil {
		return "", "", "", txId.String(), 0, err
	}

	*vm.Key = fmt.Sprintf("vm_%s_%04d", vm.Name, seqNum)
	//vm.NameはOSホスト名なので受けたものを利用
	vm.Uuid = stringPtr(txId.String())
	vm.CTime = timePtr(time.Now())
	vm.STime = timePtr(time.Now())
	//vm.Status = 1  // 状態プロビ中
	err = d.PutDataEtcd(*vm.Key, vm) // 仮想マシンのデータ登録

	return vm.HvNode, *vm.HvIpAddr, *vm.Key, *vm.Uuid, *vm.HvPort, err
}

// VMの終了とリソースの開放
func (d *Database) RemoveVmFromHV(vmKey string) error {

	// トランザクションであるべき？
	// VMをキーで取得して、ハイパーバイザーを取得
	vm, err := d.GetVmByKey(vmKey)
	if err != nil {
		return err
	}
	hv, err := d.GetHypervisorByKey(vm.HvNode)
	if err != nil {
		return err
	}
	// HVからリソースを削除
	*hv.FreeCpu = *hv.FreeCpu + *vm.Cpu
	*hv.FreeMemory = *hv.FreeMemory + *vm.Memory
	err = d.PutDataEtcd(vm.HvNode, &hv)
	if err != nil {
		return err
	}
	// VMを削除
	err = d.DelByKey(*vm.Key)
	if err != nil {
		return err
	}
	return nil
}

func (d *Database) UpdateVmState(vmkey string, state int) error {
	// ロックしたい
	vm, err := d.GetVmByKey(vmkey)
	if err != nil {
		return err
	}
	vm.Status = intPtr(state)
	err = d.PutDataEtcd(vmkey, vm)
	return err
}
