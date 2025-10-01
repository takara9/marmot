package marmot

/*
// 仮想マシンの停止
func (m *Marmot) StopVm(c *gin.Context) {
	slog.Info("stop vm", "etcd", m.EtcdUrl)
	var spec cf.VMSpec
	err := c.BindJSON(&spec)
	if err != nil {
		slog.Error("setup config", "err", err)
		return
	}
	err = m.stopVM(spec)
	if err != nil {
		slog.Error("stop vm", "err", err)
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
}

// VMの停止
func (m *Marmot) stopVM(spec cf.VMSpec) error {
	vm, err := m.Db.GetVmByKey(spec.Key)
	if err != nil {
		slog.Error("", "err", err)
		return nil
	}

	if vm.Status == db.RUNNING {
		// 仮想マシンの停止＆削除
		url := "qemu:///system"
		err = virt.StopVM(url, spec.Key)
		if err != nil {
			slog.Error("", "err", err)
		}
		// ハイパーバイザーのリソース削減保存
		hv, err := m.Db.GetHvByKey(vm.HvNode)
		if err != nil {
			slog.Error("", "err", err)
		}
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = m.Db.PutDataEtcd(hv.Key, hv)
		if err != nil {
			slog.Error("", "err", err)
		}
		// データベースの更新
		err = m.Db.UpdateVmState(spec.Key, db.STOPPED) ////////
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return nil
}

func stopRemoteVM(hvNode string, spec cf.VMSpec) error {
	byteJSON, _ := json.MarshalIndent(spec, "", "    ")
	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:8750/%s", hvNode, "stopVm")
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		return errors.New(string(body))
	}
	return nil
}
*/
