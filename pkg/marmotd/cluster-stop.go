package marmotd

/*
// クラスタ停止
func (m *Marmot) StopClusterInternal(cnf api.MarmotConfig) error {
	// リクエスト送信前にコンフィグのチェックを実施する
	if cnf.ClusterName == nil {
		return errors.New("cluster name is not set")
	}
	if cnf.VmSpec == nil {
		return errors.New("vm spec is not set")
	}

	var NotFound bool = true
	var vmKey string
	for _, spec := range *cnf.VmSpec {
		vmKey, _ = m.Db.FindByHostAndClusteName(*spec.Name, *cnf.ClusterName)
		slog.Debug("FindByHostAndClusteName()", "vmKey", vmKey, "spec.Name", *spec.Name, "cnf.ClusterName", *cnf.ClusterName)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = &vmKey
			vm, err := m.Db.GetVmByVmKey(vmKey)
			if err != nil {
				slog.Error("GetVmByVmKey()", "err", err, "vmKey=", vmKey)
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
			_, _, _, err = marmotClient.StopVirtualMachine(spec)
			if err != nil {
				slog.Error("marmotClient.StopVirtualMachine()", "err", err, "vmKey=", vmKey)
				m.Db.UpdateVmStateByKey(*vm.Key, types.ERROR) // エラー状態へ
				continue
			}
		}
	}
	if NotFound {
		slog.Info("StopClusterInternal()", "err", "NotExistVM", "vmKey", vmKey, "clusterName", *cnf.ClusterName)
		return errors.New("NotExistVM")
	}
	return nil
}
*/
