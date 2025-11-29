package db

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/lvm"
)

func intPtr(i int) *int32            { j := int32(i); return &j }
func int64Ptr(i int) *int64          { j := int64(i); return &j }
func int32Ptr(i uint64) *int32       { j := int32(i); return &j }
func int64PtrConvMB(i uint64) *int64 { j := int64(i) * 1024; return &j }
func stringPtr(s string) *string     { return &s }
func timePtr(t time.Time) *time.Time { return &t }

// ハイパーバイザーの設定
func (d *Database) SetHypervisors(v config.Hypervisor_yaml) error {
	var hv api.Hypervisor
	hv.NodeName = v.Name
	hv.Port = int32Ptr(v.Port)
	hvKey := HvPrefix + "/" + v.Name
	hv.Key = &hvKey
	hv.IpAddr = &v.IpAddr
	hv.Cpu = int32(v.Cpu) // 必須項目のためポインタではない
	hv.FreeCpu = int32Ptr(v.Cpu)
	hv.Memory = int64PtrConvMB(v.Ram)
	hv.FreeMemory = int64PtrConvMB(v.Ram)
	hv.Status = int32Ptr(2) // 暫定

	var stgpool []api.StoragePool
	for _, val := range v.Storage {
		var sp api.StoragePool
		sp.VolGroup = &val.VolGroup
		sp.Type = &val.Type
		stgpool = append(stgpool, sp)
	}
	hv.StgPool = &stgpool

	if err := d.PutDataEtcd(hvKey, hv); err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}

	return nil
}

func (d *Database) NewHypervisor(node string, hv api.Hypervisor) error {
	hv.NodeName = node
	etcdKey := HvPrefix + "/" + node
	hv.Key = &etcdKey
	hv.Status = int32Ptr(2) // 暫定

	if err := d.PutDataEtcd(etcdKey, hv); err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}

	return nil
}

// Keyに一致したHVデータの取り出し
func (d *Database) GetHypervisorByName(hbNode string) (api.Hypervisor, error) {
	var hv api.Hypervisor

	resp, err := d.Cli.Get(d.Ctx, HvPrefix+"/"+hbNode)
	if err != nil {
		slog.Error("GetHypervisorByName()", "err", err)
		return api.Hypervisor{}, err
	}

	if resp.Count == 0 {
		return hv, errors.New("not found")
	}

	if err = json.Unmarshal([]byte(resp.Kvs[0].Value), &hv); err != nil {
		slog.Error("Unmarshal()", "err", err)
		return api.Hypervisor{}, err
	}

	return hv, err
}

// Keyに一致したHVを削除
func (d *Database) DeleteHypervisorByName(name string) error {
	if err := d.DelByKey(HvPrefix + "/" + name); err != nil {
		return err
	}
	return nil
}

// ハイパーバイザーのデータを取得
func (d *Database) GetHypervisors(hvs *[]api.Hypervisor) error {
	resp, err := d.GetEtcdByPrefix(HvPrefix)
	if err != nil {
		if err.Error() == "not found" {
			return nil
		}
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
	hv, err := d.GetHypervisorByName(nodeName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	for i := 0; i < len(*hv.StgPool); i++ {
		total_sz, free_sz, err := lvm.CheckVG(*(*hv.StgPool)[i].VolGroup)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		(*hv.StgPool)[i].FreeCap = int64Ptr(int(free_sz / 1024 / 1024 / 1024))
		(*hv.StgPool)[i].VgCap = int64Ptr(int(total_sz / 1024 / 1024 / 1024))
	}

	// DBへ書き込み
	if err := d.PutDataEtcd(HvPrefix+"/"+nodeName, hv); err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}

func (d *Database) CheckHvVG2ByName(nodeName string, vg string) error {
	// LVMへのアクセス
	total_sz, free_sz, err := lvm.CheckVG(vg)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	hv, err := d.GetHypervisorByName(nodeName)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 一致するVGにデータをセット
	for i := 0; i < len(*hv.StgPool); i++ {
		if *(*hv.StgPool)[i].VolGroup == vg {
			(*hv.StgPool)[i].FreeCap = int64Ptr(int(free_sz / 1024 / 1024 / 1024))
			(*hv.StgPool)[i].VgCap = int64Ptr(int(total_sz / 1024 / 1024 / 1024))
		}
	}

	// DBへ書き込み
	err = d.PutDataEtcd(HvPrefix+"/"+nodeName, hv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}
