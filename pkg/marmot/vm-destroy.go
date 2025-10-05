package marmot

import (
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// VMの削除
func (m *Marmot) DestroyVM2(spec api.VmSpec) error {
	if spec.Key != nil {
		fmt.Println("key=", *spec.Key)
	}
	vm, err := m.Db.GetVmByKey(*spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// ハイパーバイザーのリソース削減保存のため値を取得
	hv, err := m.Db.GetHypervisorByKey(vm.HvNode)
	if err != nil {
		slog.Error("", "err", err)
	}

	// ステータスを調べて停止中であれば、足し算しない。
	if vm.Status != db.STOPPED || vm.Status == db.ERROR {
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = m.Db.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
	}

	// データベースから削除
	err = m.Db.DelByKey(*spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// 仮想マシンの停止＆削除
	url := "qemu:///system"
	err = virt.DestroyVM(url, *spec.Key)
	if err != nil {
		slog.Error("", "err", err)
	}

	// OS LVを削除
	err = lvm.RemoveLV(vm.OsVg, vm.OsLv)
	if err != nil {
		slog.Error("", "err", err)
	}
	// ストレージの更新
	util.CheckHvVG2(m.EtcdUrl, m.NodeName, vm.OsVg)

	// データLVを削除
	for _, dd := range vm.Storage {
		err = lvm.RemoveLV(dd.Vg, dd.Lv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// ストレージの更新
		util.CheckHvVG2(m.EtcdUrl, m.NodeName, dd.Vg)
	}
	return nil
}
