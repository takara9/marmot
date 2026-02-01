package db

import (
	"encoding/json"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

/*
// api.VirtualMachineに、OSボリュームのタイプ情報を追加する
// この関数で登録された場合は、OSボリュームはLV形式であることをセットする
func (d *Database) UpdateOsLvByVmKey(vmKey string, osVg string, osLv string) error {
	slog.Debug("UpdateOsLvByVmKey()", "vmKey", vmKey, "osVg", osVg, "osLv", osLv)

	lockKey := "/lock/vm/" + vmKey
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	// 名前に一致した情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		slog.Error("GetVmByVmKey()", "err", err)
		return err
	}
	slog.Debug("GetVmByVmKey()", "vmKey", vmKey, "vm.OsLv", vm.OsLv, "vm.OsVg", vm.OsVg)

	// LV, VGをセット
	vm.OsLv = util.StringPtr(osLv)
	vm.OsVg = util.StringPtr(osVg)

	// etcdへ登録
	err = d.PutVmByVmKey(vmKey, vm)
	if err != nil {
		slog.Error("PutVmByVmKey()", "err", err)
		return err
	}
	vm2, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		slog.Error("GetVmByVmKey()", "err", err)
		return err
	}
	slog.Debug("GetVmByVmKey()", "name", vm2.Name, "key", vm2.Key, "vm2.OsLv", vm2.OsLv, "vm2.OsVg", vm2.OsVg)

	return nil
}

// データボリュームLVをetcdへ登録
func (d *Database) UpdateDataLvByVmKey(vmKey string, idx int, dataVg string, dataLv string) error {
	lockKey := "/lock/vm/" + vmKey
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	// 名前に一致したVM情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}
	(*vm.Storage)[idx].Lv = util.StringPtr(dataLv)
	(*vm.Storage)[idx].Vg = util.StringPtr(dataVg)
	return d.PutVmByVmKey(vmKey, vm)
}
*/

// イメージテンプレート
func (d *Database) SetImageTemplate(v cf.Image_yaml) error {
	lockKey := "/lock/image/" + v.Name
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	key := OsTemplateImagePrefix + "/" + v.Name
	var osit types.OsImageTemplate
	osit.Key = key
	osit.LogicalVolume = v.LogicalVolume
	osit.VolumeGroup = v.VolumeGroup
	osit.OsVariant = v.Name
	osit.Qcow2Path = v.Qcow2ImagePath
	if err := d.PutJSON(key, osit); err != nil {
		slog.Error("failed to write image template data", "err", err, "key", key)
		return err
	}
	return nil
}

func (d *Database) GetOsImgTempByKey(key string) (types.OsImageTemplate, error) {
	var osit types.OsImageTemplate
	if _, err := d.GetJSON(key, &osit); err != nil {
		return types.OsImageTemplate{}, err
	}
	return osit, nil
}

// Keyに一致したOSイメージテンプレートを返す
// lvタイプとqcow2タイプの両方に対応するようにしたい
// OsImageTemplate構造体にタイプ情報を追加する必要があるかも
func (d *Database) GetOsImgTempByOsVariant(osVariant string) (types.OsImageTemplate, error) {
	slog.Debug("GetOsImgTempByOsVariant", "key=", OsTemplateImagePrefix+"/"+osVariant)
	key := OsTemplateImagePrefix + "/" + osVariant
	var osit types.OsImageTemplate

	if _, err := d.GetJSON(key, &osit); err != nil {
		return types.OsImageTemplate{}, err
	}
	return osit, nil
}

func (d *Database) GetOsImgTempes(osits *[]types.OsImageTemplate) error {
	resp, err := d.GetByPrefix(OsTemplateImagePrefix)
	if err != nil {
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
