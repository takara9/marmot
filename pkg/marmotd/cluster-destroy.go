package marmotd

/*
// クラスタ削除
func (m *Marmot) DestroyClusterInternal(cnf api.MarmotConfig) error {
	if cnf.VmSpec == nil || cnf.ClusterName == nil {
		return errors.New("VM Spec or Cluster Name is not set")
	}

	for _, spec := range *cnf.VmSpec {
		// クラスタ名とホスト名の重複チェック
		if spec.Name == nil {
			return errors.New("VM Name is not set")
		}
		vmKey, err := m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		if err == db.ErrFound {
			slog.Debug("Found VM for destroy", "vmKey", vmKey)
		} else if err != nil {
			return err
		}

		slog.Debug("DestroyClusterInternal", "vmKey==", vmKey)

		spec.Key = &vmKey
		vm, err := m.Db.GetVmByVmKey(vmKey)
		if err != nil {
			slog.Error("GetVmByVmKey()", "err", err, "vmKey", vmKey)
			continue
		}

		hvService := fmt.Sprintf("%s:%d", *vm.HvIpAddr, *vm.HvPort)
		marmotClient, err := client.NewMarmotdEp(
			"http",
			hvService,
			"/api/v1",
			15,
		)
		if err != nil {
			continue
		}

		slog.Debug("destroy virtual machine", "spec.key==", spec.Key)

		_, _, _, err = marmotClient.DestroyVirtualMachine(spec)
		if err != nil {
			slog.Error("destroy virtual machine", "err", err)
			m.Db.UpdateVmStateByKey(*vm.Key, types.ERROR) // エラー状態へ
			continue
		}
	}
	return nil
}
*/
