package db

import (
	"encoding/json"
	"errors"
	"log/slog"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
	"github.com/takara9/marmot/pkg/util"
)

// api.VirtualMachineに、OSボリュームのタイプ情報を追加する
// この関数で登録された場合は、OSボリュームはLV形式であることをセットする
func (d *Database) UpdateOsLvByVmKey(vmKey string, osVg string, osLv string) error {
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

// データボリュームLVをetcdへ登録
func (d *Database) UpdateDataLvByVmKey(vmKey string, idx int, dataVg string, dataLv string) error {
	// 名前に一致したVM情報を取得
	vm, err := d.GetVmByVmKey(vmKey)
	if err != nil {
		return err
	}
	(*vm.Storage)[idx].Lv = util.StringPtr(dataLv)
	(*vm.Storage)[idx].Vg = util.StringPtr(dataVg)
	return d.PutVmByVmKey(vmKey, vm)
}

// イメージテンプレート
func (d *Database) SetImageTemplate(v cf.Image_yaml) error {
	var osit types.OsImageTemplate
	osit.LogicalVolume = v.LogicalVolume
	osit.VolumeGroup = v.VolumeGroup
	osit.OsVariant = v.Name
	osit.Qcow2Path = v.Qcow2ImagePath
	osit.Key = OsTemplateImagePrefix + "/" + osit.OsVariant
	if err := d.PutDataEtcd(osit.Key, osit); err != nil {
		slog.Error("SetImageTemplate() put", "err", err, "key", osit.Key)
		return err
	}
	return nil
}

func (d *Database) GetOsImgTempByKey(key string) (types.OsImageTemplate, error) {
	var osit types.OsImageTemplate
	resp, err := d.Cli.Get(d.Ctx, key)
	if err != nil {
		return osit, err
	}
	if resp.Count == 0 {
		slog.Error("GetOsImgTempByKey() NotFound", "key", key)
		return osit, errors.New("NotFound")
	}

	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &osit)
	if err != nil {
		return osit, err
	}
	return osit, nil
}

// Keyに一致したOSイメージテンプレートを返す
// lvタイプとqcow2タイプの両方に対応するようにしたい
// OsImageTemplate構造体にタイプ情報を追加する必要があるかも
func (d *Database) GetOsImgTempByOsVariant(osVariant string) (types.OsImageTemplate, error) {
	slog.Debug("GetOsImgTempByOsVariant", "key=", OsTemplateImagePrefix+"/"+osVariant)
	resp, err := d.Cli.Get(d.Ctx, OsTemplateImagePrefix+"/"+osVariant)
	if err != nil {
		return types.OsImageTemplate{}, err
	}
	if resp.Count == 0 {
		slog.Error("GetOsImgTempByKey() NotFound", "key", OsTemplateImagePrefix+"/"+osVariant)
		return types.OsImageTemplate{}, errors.New("NotFound")
	}

	var osit types.OsImageTemplate
	err = json.Unmarshal([]byte(resp.Kvs[0].Value), &osit)
	if err != nil {
		return types.OsImageTemplate{}, err
	}
	return osit, nil
}

func (d *Database) GetOsImgTempes(osits *[]types.OsImageTemplate) error {
	resp, err := d.GetDataByPrefix(OsTemplateImagePrefix)
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
