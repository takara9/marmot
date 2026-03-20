package db

import (
	"encoding/json"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

// qcow2イメージテンプレート
func (d *Database) SetImageQcow2Template(v cf.Image_yaml) error {
	key := OsTemplateImagePrefix + "/" + v.Name
	lockKey := "/lock/image" + v.Name
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var osit types.OsImageTemplate
	osit.LogicalVolume = v.LogicalVolume
	osit.VolumeGroup = v.VolumeGroup
	osit.OsVariant = v.Name
	osit.Key = key
	if err := d.PutJSON(key, osit); err != nil {
		slog.Error("failed to write qcow2 of OS image template", "err", err, "key", osit.Key)
		return err
	}
	return nil
}

// Keyに一致したOSイメージテンプレートqcow2を返す
func (d *Database) GetOsImgTempQcow2ByOsVariant(osVariant string) (types.OsImageTemplate, error) {
	key := OsTemplateImagePrefix + "/" + osVariant
	slog.Debug("GetOsImgTempByOsVariant", "key=", key)

	var oit types.OsImageTemplate
	if _, err := d.GetJSON(key, &oit); err != nil {
		return types.OsImageTemplate{}, err
	}

	return oit, nil
}

func (d *Database) GetOsImgQcow2Tempes(osits *[]types.OsImageTemplate) error {
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
