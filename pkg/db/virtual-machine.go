package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/types"
	"github.com/takara9/marmot/pkg/util"
)

func (d *Database) GetVmByVmKey(vmKey string) (api.VirtualMachine, error) {
	if len(vmKey) == 0 {
		slog.Debug("GetVmByVmKey()", "vmKey is empty then return error", 0)
		return api.VirtualMachine{}, errors.New("not found")
	}

	resp, err := d.Cli.Get(d.Ctx, vmKey)
	if err != nil {
		return api.VirtualMachine{}, err
	}
	if resp.Count == 0 {
		return api.VirtualMachine{}, errors.New("not found")
	}

	var vm api.VirtualMachine
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &vm)
	return vm, err
}

// キーに一致したVM情報をetcdへ登録
func (d *Database) PutVmByVmKey(vmKey string, vm api.VirtualMachine) error {
	return d.PutDataEtcd(vmKey, vm)
}

// 仮想マシンのデータを取得
func (d *Database) GetVmsStatuses(vms *[]api.VirtualMachine) error {
	resp, err := d.GetEtcdByPrefix(VmPrefix)
	if err != nil {
		return err
	} else if resp.Count == 0 {
		return nil
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
	slog.Debug("=== AssignHvforVm called ===", "start", vm)
	var txId = uuid.New()

	//トランザクション開始、他更新ロック 仮想マシンをデータベースに登録、状態は「データ登録中」
	var hvs []api.Hypervisor
	if err := d.GetHypervisors(&hvs); err != nil { // HVのステータス取得
		return "", "", "", txId.String(), 0, err
	}
	slog.Debug("=== AssignHvforVm", "d.GetHypervisors()", hvs)

	// フリーのCPU数の降順に並べ替える
	sort.Slice(hvs, func(i, j int) bool { return *hvs[i].FreeCpu > *hvs[j].FreeCpu })
	slog.Debug("=== AssignHvforVm", "sorted", hvs)

	// リソースに空きのあるハイパーバイザーを探す
	var assigned = false
	var hv api.Hypervisor

	for _, hv = range hvs {
		// 停止中のHVの割り当てない
		if *hv.Status != types.RUNNING {
			continue
		}

		if *hv.FreeCpu >= *vm.Cpu {
			if *hv.FreeMemory >= *vm.Memory {
				slog.Debug("=== AssignHvforVm assigned", "hv=", hv)
				*hv.FreeMemory = *hv.FreeMemory - *vm.Memory
				*hv.FreeCpu = *hv.FreeCpu - *vm.Cpu
				// ストレージの容量管理は未実装
				vm.Status = util.Int64PtrInt32(types.INITIALIZING) // 登録中
				vm.HvNode = hv.NodeName                            // ハイパーバイザーを決定
				vm.HvIpAddr = hv.IpAddr
				vm.HvPort = hv.Port
				assigned = true
				break
			}
		}
	}
	// リソースに空きが無い場合はエラーを返す
	if !assigned {
		slog.Debug("=== AssignHvforVm failed to assign", "", "")
		err := errors.New("could't assign VM due to doesn't have enough a resouce on HV")
		return "", "", "", txId.String(), 0, err
	}

	// ハイパーバイザーのリソース削減保存
	//etcdKey := HvPrefix + "/" + *hv.Key
	if err := d.PutDataEtcd(*hv.Key, hv); err != nil {
		return "", "", "", txId.String(), 0, err
	}
	slog.Debug("=== d.PutDataEtcd", "hv.Key", *hv.Key)

	// VM名登録　シリアル番号取得
	seqNum, err := d.GetSeqByKind("VM")
	if err != nil {
		return "", "", "", txId.String(), 0, err
	}
	slog.Debug("=== d.GetSeq()", "seqNum", seqNum)

	//vm.NameはOSホスト名なので受けたものを利用
	vm.Key = util.StringPtr(fmt.Sprintf("%v/%s_%04d", VmPrefix, vm.Name, seqNum))
	vm.Uuid = util.StringPtr(txId.String())
	vm.CTime = util.TimePtr(time.Now())
	vm.STime = util.TimePtr(time.Now())
	vm.Status = util.Int64PtrInt32(types.PROVISIONING)  // プロビジョニング中
	if err := d.PutDataEtcd(*vm.Key, &vm); err != nil { // 仮想マシンのデータ登録
		slog.Debug("=== d.PutDataEtcd failed", "vm.Key", *vm.Key, "err", err)
		return "", "", "", txId.String(), 0, err
	}

	slog.Debug("=== d.PutDataEtcd", "vm.Key", *vm.Key, "err", err)

	return vm.HvNode, *vm.HvIpAddr, *vm.Key, txId.String(), *vm.HvPort, err
}

func (d *Database) UpdateVmStateByKey(vmKey string, state int) error {
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}
	vm.Status = util.IntPtrInt32(state)
	err = d.PutDataEtcd(vmKey, vm)
	return err
}
