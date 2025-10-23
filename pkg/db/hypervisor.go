package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	. "github.com/takara9/marmot/pkg/types"
)

// ハイパーバイザーの設定
func (d *Database) SetHypervisors(v cf.Hypervisor_yaml) error {
	var hv Hypervisor
	hv.Nodename = v.Name
	hv.Port = int(v.Port)
	hv.Key = v.Name // Key
	hv.IpAddr = v.IpAddr
	hv.Cpu = int(v.Cpu)
	hv.FreeCpu = int(v.Cpu)       // これで良いのか
	hv.Memory = int(v.Ram * 1024) // MB
	hv.FreeMemory = int(v.Ram * 1024)
	hv.Status = 2 // テストのため暫定
	for _, val := range v.Storage {
		var sp StoragePool
		sp.VolGroup = val.VolGroup
		sp.Type = val.Type
		hv.StgPool = append(hv.StgPool, sp)
	}
	err := d.PutDataEtcd(hv.Key, hv)
	if err != nil {
		slog.Error("PutDataEtcd()", "err", err)
		return err
	}
	return nil

}

// Keyに一致したHVデータの取り出し
func (d *Database) GetHypervisorByKey(key string) (Hypervisor, error) {
	var hv Hypervisor

	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return hv, err
	}

	if resp.Count == 0 {
		return hv, errors.New("not found")
	}
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &hv)

	return hv, err
}

// ハイパーバイザーのデータを取得
func (d *Database) GetHypervisors(hvs *[]Hypervisor) error {
	resp, err := d.GetEtcdByPrefix("hv")
	if err != nil {
		return err
	}
	for _, ev := range resp.Kvs {
		var hv Hypervisor
		err = json.Unmarshal([]byte(ev.Value), &hv)
		if err != nil {
			return err
		}
		*hvs = append(*hvs, hv)
	}
	return nil
}

// ハイパーバイザーのデータを取得 今後削除予定
func (d *Database) GetHypervisorsOld(hvs *[]HypervisorOld) error {
	resp, err := d.GetEtcdByPrefix("hv")
	if err != nil {
		return err
	}
	for _, ev := range resp.Kvs {
		var hv HypervisorOld
		err = json.Unmarshal([]byte(ev.Value), &hv)
		if err != nil {
			return err
		}
		*hvs = append(*hvs, hv)
	}
	return nil
}
