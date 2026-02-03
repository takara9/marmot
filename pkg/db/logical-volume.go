package db

import (
	"encoding/json"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

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
