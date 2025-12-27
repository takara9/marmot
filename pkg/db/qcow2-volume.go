package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
	"github.com/takara9/marmot/pkg/util"
)

// OS qcow2ボリューム情報をetcdへ登録
func (d *Database) UpdateOsQcow2ByVmKey(vmKey string, osVg string, osLv string) error {
	// 名前に一致した情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}

	// LV, VGをセット
	vm.OsLv = util.StringPtr(osLv)
	vm.OsVg = util.StringPtr(osVg)

	// etcdへ登録
	return d.PutVmByVmKey(vmKey, vm)
}

// データqcow2ボリュームをetcdへ登録
func (d *Database) UpdateDataQcow2LvByVmKey(vmKey string, idx int, dataVg string, dataLv string) error {
	// 名前に一致したVM情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}
	(*vm.Storage)[idx].Lv = util.StringPtr(dataLv)
	(*vm.Storage)[idx].Vg = util.StringPtr(dataVg)
	return d.PutVmByVmKey(vmKey, vm)
}

// qcow2イメージテンプレート
func (d *Database) SetImageQcow2Template(v cf.Image_yaml) error {
	var osit types.OsImageTemplate
	osit.LogicalVolume = v.LogicalVolume
	osit.VolumeGroup = v.VolumeGroup
	osit.OsVariant = v.Name
	key := OsTemplateImagePrefix + "/" + osit.OsVariant
	osit.Key = key

	lockKey := "/lock/image/" + osit.OsVariant
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	if err := d.PutJSON(key, osit); err != nil {
		slog.Error("failed to write template image data", "err", err, "key", osit.Key)
		return err
	}
	return nil
}

// Keyに一致したOSイメージテンプレートqcow2を返す
func (d *Database) GetOsImgTempQcow2ByOsVariant(osVariant string) (types.OsImageTemplate, error) {
	key := OsTemplateImagePrefix + "/" + osVariant
	slog.Debug("GetOsImgTempByOsVariant", "key=", key)

	var osit types.OsImageTemplate
	_, err := d.GetJSON(key, &osit)
	if err == nil {
		return osit, nil
	}
	return osit, nil
}

func (d *Database) GetOsImgQcow2Tempes(osits *[]types.OsImageTemplate) error {
	resp, err := d.GetByPrefix(OsTemplateImagePrefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
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
