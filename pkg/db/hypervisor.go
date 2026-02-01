package db

/*
// ハイパーバイザーの設定
func (d *Database) SetHypervisors(v config.Hypervisor_yaml) error {
	key := HvPrefix + "/" + v.Name
	lockKey := "/lock/hv/" + v.Name
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var hv api.Hypervisor
	hv.NodeName = v.Name
	hv.Port = util.Int64PtrInt32(v.Port)
	hv.Key = &key
	hv.IpAddr = &v.IpAddr
	hv.Cpu = int32(v.Cpu)
	hv.FreeCpu = util.Int64PtrInt32(v.Cpu)
	hv.Memory = util.Int64PtrConvMB(v.Ram)
	hv.FreeMemory = util.Int64PtrConvMB(v.Ram)
	hv.Status = util.Int64PtrInt32(2) // 暫定

	var stgpool []api.StoragePool
	for _, val := range v.Storage {
		var sp api.StoragePool
		sp.VolGroup = &val.VolGroup
		sp.Type = &val.Type
		stgpool = append(stgpool, sp)
	}
	hv.StgPool = &stgpool

	if err := d.PutJSON(key, hv); err != nil {
		slog.Error("failed to write hypervisor data", "err", err)
		return err
	}

	return nil
}

func (d *Database) NewHypervisor(node string, hv api.Hypervisor) error {
	key := HvPrefix + "/" + node
	lockKey := "/lock/hv/" + node
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	hv.NodeName = node
	hv.Key = &key
	hv.Status = util.Int64PtrInt32(2) // 暫定

	if err := d.PutJSON(key, hv); err != nil {
		slog.Error("failed to write hypervisor data", "err", err, "key", key)
		return err
	}

	return nil
}

// Keyに一致したHVデータの取り出し
func (d *Database) GetHypervisorByName(nodeName string) (api.Hypervisor, error) {
	var hv api.Hypervisor
	key := HvPrefix + "/" + nodeName

	if _, err := d.GetJSON(key, &hv); err != nil {
		slog.Error("failed to get hypervisor by name", "err", err)
		return hv, err
	}

	return hv, nil
}

// Keyに一致したHVを削除
func (d *Database) DeleteHypervisorByName(nodeName string) error {
	key := HvPrefix + "/" + nodeName
	lockKey := "/lock/hv/" + nodeName
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	if err := d.DeleteJSON(key); err != nil {
		return err
	}
	return nil
}

// ハイパーバイザーのデータを取得
func (d *Database) GetHypervisors(hvs *[]api.Hypervisor) error {
	resp, err := d.GetByPrefix(HvPrefix)
	if err != nil {
		return err
	}

	for _, ev := range resp.Kvs {
		var hv api.Hypervisor
		err = json.Unmarshal([]byte(ev.Value), &hv)
		if err != nil {
			return err
		}
		*hvs = append(*hvs, hv)
	}
	return nil
}

func (d *Database) CheckHvVgAllByName(nodeName string) error {
	key := HvPrefix + "/" + nodeName
	lockKey := "/lock/hv/" + nodeName
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	hv, err := d.GetHypervisorByName(nodeName)
	if err != nil {
		slog.Error("failed to get hypervisor by name", "err", err)
		return err
	}

	for i := 0; i < len(*hv.StgPool); i++ {
		total_sz, free_sz, err := lvm.CheckVG(*(*hv.StgPool)[i].VolGroup)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		(*hv.StgPool)[i].FreeCap = util.IntPtrInt64(int(free_sz / 1024 / 1024 / 1024)) //GBに変換
		(*hv.StgPool)[i].VgCap = util.IntPtrInt64(int(total_sz / 1024 / 1024 / 1024))
	}

	if err := d.PutJSON(key, hv); err != nil {
		slog.Error("failed to write hypervisor data", "err", err)
		return err
	}
	return nil
}

func (d *Database) CheckHvVG2ByName(nodeName string, vg string) error {
	key := HvPrefix + "/" + nodeName
	lockKey := "/lock/hv/" + nodeName
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	// LVMへのアクセス
	total_sz, free_sz, err := lvm.CheckVG(vg)
	if err != nil {
		slog.Error("failed to check VG", "err", err)
		return err
	}

	hv, err := d.GetHypervisorByName(nodeName)
	if err != nil {
		slog.Error("failed to get hypervisor by name", "err", err)
		return err
	}

	// 一致するVGにデータをセット
	for i := 0; i < len(*hv.StgPool); i++ {
		if *(*hv.StgPool)[i].VolGroup == vg {
			(*hv.StgPool)[i].FreeCap = util.IntPtrInt64(int(free_sz / 1024 / 1024 / 1024))
			(*hv.StgPool)[i].VgCap = util.IntPtrInt64(int(total_sz / 1024 / 1024 / 1024))
		}
	}

	if err := d.PutJSON(key, hv); err != nil {
		slog.Error("failed to write hypervisor data", "err", err)
		return err
	}
	return nil
}

// ハイパーバイザーをREST-APIでアクセスして疎通を確認、DBへ反映させる
func (d *Database) CheckHypervisors(dbUrl string, node string) ([]api.Hypervisor, error) {
	lockKey := "/lock/hvs"
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return nil, err
	}
	defer d.UnlockKey(mutex)

	var hvs []api.Hypervisor
	if err := d.GetHypervisors(&hvs); err != nil {
		slog.Error("failed to get hypervisors", "err", err, "node", node)
		return nil, err
	}

	for _, val := range hvs {
		if err := d.PutJSON(*val.Key, val); err != nil {
			slog.Error("failed to write hypervisor data", "err", err, "key", *val.Key)
			return nil, err
		}
	}
	return hvs, nil
}
*/
