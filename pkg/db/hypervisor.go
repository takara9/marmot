package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
)

func int32Ptr(i uint64) *int32       { j := int32(i); return &j }
func int64PtrConvMB(i uint64) *int64 { j := int64(i) * 1024; return &j }
func stringPtr(s string) *string     { return &s }

// ハイパーバイザーの設定
func (d *Database) SetHypervisors(v config.Hypervisor_yaml) error {
	var hv api.Hypervisor

	hv.NodeName = v.Name
	hv.Port = int32Ptr(v.Port)
	hv.Key = &v.Name // Key
	hv.IpAddr = &v.IpAddr
	hv.Cpu = int32(v.Cpu) // 必須項目のためポインタではない
	hv.FreeCpu = int32Ptr(v.Cpu)
	hv.Memory = int64PtrConvMB(v.Ram)
	hv.FreeMemory = int64PtrConvMB(v.Ram)
	hv.Status = int32Ptr(2) // 暫定

	if len(v.Storage) > 0 {
		for _, val := range v.Storage {
			var sp api.StoragePool
			sp.VolGroup = &val.VolGroup
			sp.Type = &val.Type
			*hv.StgPool = append(*hv.StgPool, sp)
		}
	}
	err := d.PutDataEtcd(*hv.Key, hv)
	if err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}
	return nil
}

// Keyに一致したHVデータの取り出し
func (d *Database) GetHypervisorByKey(key string) (api.Hypervisor, error) {
	var hv api.Hypervisor
	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return api.Hypervisor{}, err
	}
	if resp.Count == 0 {
		return hv, errors.New("not found")
	}
	if err = json.Unmarshal([]byte(resp.Kvs[0].Value), &hv); err != nil {
		return api.Hypervisor{}, err
	}
	return hv, err
}

// ハイパーバイザーのデータを取得
func (d *Database) GetHypervisors(hvs *[]api.Hypervisor) error {
	resp, err := d.GetEtcdByPrefix("hv")
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

/*
// ハイパーバイザーのデータを取得 今後削除予定
func (d *Database) GetHypervisorsOld(hvs *[]types.HypervisorOld) error {
	resp, err := d.GetEtcdByPrefix("hv")
	if err != nil {
		return err
	}
	for _, ev := range resp.Kvs {
		var hv types.HypervisorOld
		err = json.Unmarshal([]byte(ev.Value), &hv)
		if err != nil {
			return err
		}
		*hvs = append(*hvs, hv)
	}
	return nil
}
*/
