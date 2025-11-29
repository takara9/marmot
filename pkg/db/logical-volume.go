package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

func (d *Database) UpdateOsLvByVmKey(vmKey string, osVg string, osLv string) error {
	// 名前に一致した情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}

	// LV, VGをセット
	vm.OsLv = stringPtr(osLv)
	vm.OsVg = stringPtr(osVg)

	// etcdへ登録
	return d.PutVmByVmKey(vmKey, vm)
}

// データボリュームLVをetcdへ登録
func (d *Database) UpdateDataLvByVmKey(vmKey string, idx int, dataVg string, dataLv string) error {
	// 名前に一致したVM情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}
	(*vm.Storage)[idx].Lv = stringPtr(dataLv)
	(*vm.Storage)[idx].Vg = stringPtr(dataVg)
	return d.PutVmByVmKey(vmKey, vm)
}

// イメージテンプレート
func (d *Database) SetImageTemplate(v cf.Image_yaml) error {
	var osit types.OsImageTemplate
	osit.LogicalVolume = v.LogicalVolume
	osit.VolumeGroup = v.VolumeGroup
	osit.OsVariant = v.Name

	if err := d.PutDataEtcd(OsImagePrefix+"/"+osit.OsVariant, osit); err != nil {
		slog.Error("SetImageTemplate() put", "err", err, "key", OsImagePrefix+"/"+osit.OsVariant)
		return err
	}
	return nil
}

// Keyに一致したOSイメージテンプレートを返す
func (d *Database) GetOsImgTempByOsVariant(osVariant string) (string, string, error) {
	slog.Debug("GetOsImgTempByOsVariant", "key=", OsImagePrefix+"/"+osVariant)
	resp, err := d.Cli.Get(d.Ctx, OsImagePrefix+"/"+osVariant)
	if err != nil {
		return "", "", err
	}
	if resp.Count == 0 {
		slog.Error("GetOsImgTempByKey() NotFound", "key", OsImagePrefix+"/"+osVariant)
		return "", "", errors.New("NotFound")
	}

	var osit types.OsImageTemplate
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &osit)
	if err != nil {
		return "", "", err
	}
	return osit.VolumeGroup, osit.LogicalVolume, nil
}

func (d *Database) GetOsImgTempes(osits *[]types.OsImageTemplate) error {
	resp, err := d.GetEtcdByPrefix(OsImagePrefix)
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
