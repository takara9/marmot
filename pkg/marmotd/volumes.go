package marmotd

// ボリュームの情報管理の関数群

import (
	"errors"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
)

//ボリューム生成
/*
	qcow2形式、lvm形式、raw形式のいずれかでボリュームを作成する。
	パラメータとして、ボリュームタイプ、サイズ、その他オプションを指定する。
	成功した場合は新規ボリュームのIDを返す。

	データベースへの登録のための構造体と
	APIで受け取る構造体が異なる点が問題と思う

	OSとデータの各ボリュームは、同キー体系で管理する


*/

func (m *Marmot) CreateVolume(v api.Volume) (string, error) {
	slog.Debug("CreateVolume()", "name", v.Name, "type", *v.Type, "kind", *v.Kind)
	// データベースにボリューム情報を登録
	vc, err := db.NewVolumeController(m.EtcdUrl)
	if err != nil {
		return "", err
	}

	// 内容が設定されていない時はデフォルト値をセットする
	volName := v.Name // ボリューム名は必須 ボリューム名はラベルとして利用、ユニークである必要はない？
	volType := util.OrDefault(v.Type, "lvm")
	volKind := util.OrDefault(v.Kind, "os")
	volSize := util.OrDefault(v.Size, 0)
	volPath := util.OrDefault(v.Path, "") // パスはタイプと種類で決まるため、空で初期化

	// ボリュームの基本情報をデータベースに登録
	volKey, err := vc.CreateVolume(volName, volPath, volType, volKind, volSize)
	if err != nil {
		return "", err
	}
	slog.Debug("ボリュームを登録", "volKey", volKey, "volType", volType, "volKind", volKind)

	// ボリュームの実体を作成
	switch volType {
	case "qcow2":
		switch volKind {
		case "os":
			return "", errors.New("unsupported volume kind and type")
		case "data":
			return "", errors.New("unsupported volume kind and type")
		default:
			return "", errors.New("unsupported volume kind")
		}
	case "lvm":
		switch volKind {
		case "os":
			slog.Debug("OSボリュームの生成", "volKind", volKind, "volKey", volKey)
			img, err := vc.Database.GetOsImgTempByOsVariant(*v.OsName)
			if err != nil {
				slog.Error("failed to get os image template", "err", err, "os_name", *v.OsName)
				return "", err
			}

			slog.Debug("OSボリュームをスナップショットで生成", "volKind", volKind)
			lvName, err := vc.Database.CreateOsLv(img.VolumeGroup, img.LogicalVolume)
			if err != nil {
				slog.Error("failed to create OS logical volume", "err", err, "vg", img.VolumeGroup, "lv", img.LogicalVolume)
				return "", err
			}

			slog.Debug("OSボリュームののVGとLVでDBを更新", "Vol Key", volKey, "LV Name", lvName, "VG Name", img.VolumeGroup) // 取得したLV名をデータベースの登録
			vol := db.Volume{
				VolumeGroup:   &img.VolumeGroup,
				LogicalVolume: &lvName,
				Status:        func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
			}
			if err := vc.UpdateVolume(volKey, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volKey", volKey)
				return "", err
			}
			return volKey, nil

		case "data":
			dataVg := "vg2" // データボリューム用のボリュームグループ名を指定　現在は固定値
			slog.Info("Dataボリュームグループを使用", "vg", dataVg)

			lvName, err := vc.Database.CreateDataLv(uint64(volSize), dataVg)
			if err != nil {

				slog.Error("failed to create Data logical volume", "err", err, "vg", dataVg, "size", volSize)
				return "", err
			}
			slog.Debug("Dataボリュームの生成 成功", "LV Name", lvName, "VG Name", dataVg, "Size", volSize)

			// 取得したLV名とサイズで、データベースを更新
			vol := db.Volume{
				VolumeName: &lvName, // 不要では？
				Status:     func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
				VolumeGroup: &dataVg,
				LogicalVolume: &lvName,
				Size: &volSize,
			}
			if err = vc.UpdateVolume(volKey, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volKey", volKey)
				return "", err
			}
			slog.Debug("Dataボリュームの情報更新 成功", "volKey", volKey)
			return volKey, nil

		default:
			return "", errors.New("unsupported volume kind")
		}

	case "raw":
		return "", errors.New("unsupported volume type")
	default:
		return "", errors.New("unsupported volume type")
	}

	// データベースの状態を更新
	//return nil
}

// ボリューム削除,API, RemoveVolume(volId)
func (m *Marmot) RemoveVolume(volId string) error {
	// データベースからボリューム情報を取得
	vc, err := db.NewVolumeController(m.EtcdUrl)
	if err != nil {
		return err
	}
	vol, err := vc.GetVolumeByKey(volId)
	if err != nil {
		return err
	}

	slog.Debug("Removing Logical volume", "volId", volId)
	slog.Debug("論理ボリューム情報取得", "LogicalVolume", vol.LogicalVolume, "VolumeGroup", vol.VolumeGroup)

	lvName := *vol.LogicalVolume
	vgName := *vol.VolumeGroup

	

	// 物理的なボリュームの削除
	if err := lvm.RemoveLV(vgName, lvName); err != nil {
		slog.Error("lvm.RemoveLV()", "err", err)
	}
	if err := vc.DeleteVolume(volId); err != nil {
		slog.Error("vc.DeleteVolume()", "err", err)
		return err
	}
	return nil
}

// ボリュームのリスト,API, GetVolumes(volType)
func GetVolumes(volType string) ([]string, error) {
	return nil, errors.New("not implemented")
}

// ボリュームの拡張,API, ExpandVolume(volId, newSize)
func ExpandVolume(volId string, newSize int) error {
	return errors.New("not implemented")
}

// ボリュームの仮想マシンへのアタッチとデタッチ,API, AttachVol(vmId, volId), DetachVol(vmId, volId)
func AttachVol(vmId string, volId string) error {
	return errors.New("not implemented")
}

func DetachVol(vmId string, volId string) error {
	return errors.New("not implemented")
}

// ボリュームの複製,API, CopyVolume(volId)
func CopyVolume(volId string) error {
	return errors.New("not implemented")
}
