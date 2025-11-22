package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

// OSボリュームのLVをetcdへ登録
func (d *Database) UpdateOsLv(vmkey string, vg string, lv string) error {
	// ロックしたい
	vm, err := d.GetVmByKey(vmkey)
	if err != nil {
		return err
	}
	vm.OsLv = stringPtr(lv)
	vm.OsVg = stringPtr(vg)
	return d.PutDataEtcd(vmkey, vm)
}

// データボリュームLVをetcdへ登録
func (d *Database) UpdateDataLv(vmkey string, idx int, vg string, lv string) error {
	// ロックしたい
	vm, err := d.GetVmByKey(vmkey)
	if err != nil {
		return err
	}
	(*vm.Storage)[idx].Lv = stringPtr(lv)
	(*vm.Storage)[idx].Vg = stringPtr(vg)
	return d.PutDataEtcd(vmkey, vm)
}

// イメージテンプレート
func (d *Database) SetImageTemplate(v cf.Image_yaml) error {
	var osi types.OsImageTemplate
	osi.LogicaVol = v.LogicalVolume
	osi.VolumeGroup = v.VolumeGroup
	osi.OsVariant = v.Name
	key := fmt.Sprintf("%v_%v", "OSI", osi.OsVariant)
	err := d.PutDataEtcd(key, osi)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil
}

// Keyに一致したOSイメージテンプレートを返す
func (d *Database) GetOsImgTempByKey(osv string) (string, string, error) {
	key := fmt.Sprintf("OSI_%v", osv)
	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return "", "", err
	}

	if resp.Count == 0 {
		return "", "", errors.New("NotFound")
	}

	var oit types.OsImageTemplate
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &oit)
	if err != nil {
		return "", "", err
	}
	return oit.VolumeGroup, oit.LogicaVol, nil
}

func (d *Database) GetOsImgTempes(osits *[]types.OsImageTemplate) error {
	resp, err := d.GetEtcdByPrefix("OSI_")
	if err != nil {
		return err
	}
	if resp.Count == 0 {
		return errors.New("NotFound")
	}

	for _, ev := range resp.Kvs {
		var osit types.OsImageTemplate
		err = json.Unmarshal(ev.Value, &osit)
		if err != nil {
			return err
		}
		*osits = append(*osits, osit)
	}

	return nil
}
