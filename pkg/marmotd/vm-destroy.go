package marmotd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/types"
	"github.com/takara9/marmot/pkg/virt"
)

// VMの削除
func (m *Marmot) DestroyVM2(spec api.VmSpec) error {
	if spec.Key != nil {
		slog.Debug("DestroyVM2()", "key", *spec.Key)
	}
	vm, err := m.Db.GetVmByVmKey(*spec.Key)
	if err != nil {
		slog.Error("GetVmByVmKey()", "err", err)
	}

	// ハイパーバイザーのリソース削減保存のため値を取得
	hv, err := m.Db.GetHypervisorByName(vm.HvNode)
	if err != nil {
		slog.Error("GetHypervisorByName()", "err", err)
	}

	// ステータスを調べて停止中であれば、足し算しない。
	if *vm.Status != types.STOPPED && *vm.Status != types.ERROR {
		*hv.FreeCpu = *hv.FreeCpu + int32(*vm.Cpu)
		*hv.FreeMemory = *hv.FreeMemory + *vm.Memory
		err = m.Db.PutDataEtcd(*hv.Key, hv)
		if err != nil {
			slog.Error("PutDataEtcd()", "err", err)
		}
	}

	fmt.Println("======= DestroyVM2()", "vmKey", *spec.Key)

	// データベースから削除
	if err := m.Db.DelByKey(*spec.Key); err != nil {
		slog.Error("DelByKey(", "err", err)
	}

	domName := strings.Split(*spec.Key, "/")
	// 仮想マシンの停止＆削除
	if err := virt.DestroyVM("qemu:///system", domName[len(domName)-1]); err != nil {
		slog.Error("DestroyVM()", "err", err, "vmKey", *spec.Key, "key", domName[len(domName)-1])
	}

	// OS LVを削除
	if err := lvm.RemoveLV(*vm.OsVg, *vm.OsLv); err != nil {
		slog.Error("lvm.RemoveLV()", "err", err)
	}

	// ストレージの更新
	m.Db.CheckHvVG2ByName(m.NodeName, *vm.OsVg)

	// データLVを削除
	if vm.Storage != nil {
		for _, dd := range *vm.Storage {
			err = lvm.RemoveLV(*dd.Vg, *dd.Lv)
			if err != nil {
				slog.Error("RemoveLV()", "err", err)
			}
			// ストレージの更新
			m.Db.CheckHvVG2ByName(m.NodeName, *dd.Vg)
		}
	}
	return nil
}
