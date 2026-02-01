package marmotd

/*
// VMの削除
func (m *Marmot) DestroyVM2(spec api.VmSpec) error {
	slog.Debug("===", "DestroyVM2 is called", "===")
	if spec.Key != nil {
		slog.Debug("DestroyVM2()", "key", *spec.Key)
	}

	var vm api.VirtualMachine
	var err error

	if spec.Key != nil {
		vm, err = m.Db.GetVmByVmKey(*spec.Key)
		if err != nil {
			slog.Error("GetVmByVmKey()", "err", err)
		}
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
		err = m.Db.PutJSON(*hv.Key, hv)
		if err != nil {
			slog.Error("PutDataEtcd()", "err", err)
		}
	}

	slog.Debug("DestroyVM2() proceed to delete VM on database", "vmKey", *spec.Key)
	// データベースからVMを削除
	if err := m.Db.DeleteJSON(*spec.Key); err != nil {
		slog.Error("DeleteJSON(", "err", err)
	}

	// 仮想マシンの停止＆削除
	domName := strings.Split(*spec.Key, "/")
	slog.Debug("DestroyVM2() proceed to delete VM on hypervisor", "vmKey", *spec.Key, "domName", domName[len(domName)-1])

	if err := virt.DestroyVM("qemu:///system", domName[len(domName)-1]); err != nil {
		slog.Error("DestroyVM()", "err", err, "vmKey", *spec.Key, "key", domName[len(domName)-1])
	}

	// OS LVを削除
	slog.Debug("DestroyVM2() proceed to delete OS LV", "vm.OsVg", *vm.OsVg, "vm.OsLv", *vm.OsLv)
	if err := lvm.RemoveLV(*vm.OsVg, *vm.OsLv); err != nil {
		slog.Error("lvm.RemoveLV()", "err", err)
	}

	// ストレージの更新
	m.Db.CheckHvVG2ByName(m.NodeName, *vm.OsVg)

	// データLVを削除
	if vm.Storage != nil {
		for _, dd := range *vm.Storage {
			slog.Debug("DestroyVM2() proceed to delete Data LV", "dd.Vg", *dd.Vg, "dd.Lv", *dd.Lv)
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
*/
