package marmotd

// ボリュームの情報管理の関数群

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/qcow"
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
//
func (m *Marmot) CreateVolume(v api.Volume) (string, error) {
	slog.Debug("CreateVolume()", "name", v.Name, "type", *v.Type, "kind", *v.Kind)

	// 内容が設定されていない時はデフォルト値をセットする
	volName := v.Name // ボリューム名は必須 ボリューム名はラベルとして利用、ユニークである必要はない？
	volType := util.OrDefault(v.Type, "lvm")
	volKind := util.OrDefault(v.Kind, "os")
	volSize := util.OrDefault(v.Size, 0)
	volPath := util.OrDefault(v.Path, "") // パスはタイプと種類で決まるため、空で初期化

	// ボリュームの基本情報をデータベースに登録
	volKey, err := m.Vc.CreateVolumeOnDB(volName, volPath, volType, volKind, volSize)
	if err != nil {
		return "", err
	}
	//defer vc.RollbackVolumeCreation(volKey, err)
	slog.Debug("ボリュームを登録", "volKey", volKey, "volType", volType, "volKind", volKind)

	// ボリュームの実体を作成
	switch volType {
	case "qcow2":
		switch volKind {
		case "os":
			slog.Debug("qcow2 OSボリュームの生成", "volKind", volKind, "volKey", volKey)
			if v.OsName == nil {
				slog.Error("OsName is required for os volume")
				m.Vc.RollbackVolumeCreation(volKey)
				return "", errors.New("OsName is required for os volume")
			}
			// OS名から該当するOSイメージテンプレートを取得
			img, err := m.Vc.Database.GetOsImgTempByOsVariant(*v.OsName)
			if err != nil {
				slog.Error("failed to get os image template", "err", err)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}
			if len(img.Qcow2Path) == 0 {
				slog.Error("os image template has no qcow2 path", "len(img.Qcow2Path)", len(img.Qcow2Path))
				m.Vc.RollbackVolumeCreation(volKey)
				return "", errors.New("os image template has no qcow2 path")
			}

			slog.Debug("qcow2 OSボリュームをテンプレートから生成", "テンプレートパス", img.Qcow2Path)
			// テンプレートからOSボリュームを生成する
			seq, err := m.Vc.Database.GetSeqByKind("LVOS")
			if err != nil {
				return "", err
			}
			qcow2Name := fmt.Sprintf("qcow2-%04d", seq)

			// パスを設定する場所を作るべきか？
			qcow2Path := fmt.Sprintf("/var/lib/marmot/volumes/%s.qcow2", qcow2Name)
			slog.Debug("新規qcow2ボリュームのパス決定", "qcow2Path", qcow2Path)

			err = qcow.CopyQcow(img.Qcow2Path, qcow2Path)
			if err != nil {
				slog.Error("failed to copy qcow2 image template", "err", err, "src", img.Qcow2Path, "dst", qcow2Path)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}
			return volKey, nil
		case "data":
			slog.Debug("qcow2 Dataボリュームの生成", "volKind", volKind, "volKey", volKey)

			// データベースへの登録は済んでいるので、qcow2ファイルを新規作成
			seq, err := m.Vc.Database.GetSeqByKind("LVDATA")
			if err != nil {
				return "", err
			}
			qcow2Name := fmt.Sprintf("qcow2-%04d", seq)
			qcow2Path := fmt.Sprintf("/var/lib/marmot/volumes/%s.qcow2", qcow2Name)
			slog.Debug("新規qcow2ボリュームのパス決定", "qcow2Path", qcow2Path)
			err = qcow.CreateQcow(qcow2Path, volSize)
			if err != nil {
				slog.Error("failed to create qcow2 data volume", "err", err, "path", qcow2Path)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}
			// 取得したqcow2パスで、データベースを更新
			// 取得したLV名とサイズで、データベースを更新
			vol := db.Volume{
				Status: func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
				Path:   &qcow2Path,
				Size:   &volSize,
			}
			if err = m.Vc.UpdateVolume(volKey, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volKey", volKey)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}

			slog.Debug("Dataボリュームの情報更新 成功", "volKey", volKey)
			return volKey, nil

		default:
			err := errors.New("unsupported unknown volume kind and type")
			m.Vc.RollbackVolumeCreation(volKey)
			return "", err
		}
	case "lvm":
		switch volKind {
		case "os":
			slog.Debug("LV OSボリュームの生成", "volKind", volKind, "volKey", volKey)
			img, err := m.Vc.Database.GetOsImgTempByOsVariant(*v.OsName)
			if err != nil {
				slog.Error("failed to get os image template", "err", err, "os_name", *v.OsName)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}

			slog.Debug("OSボリュームをスナップショットで生成", "volKind", volKind)
			seq, err := m.Vc.Database.GetSeqByKind("LVOS")
			if err != nil {
				return "", err
			}
			lvName := fmt.Sprintf("oslv%04d", seq)
			var lvSize uint64 = 1024 * 1024 * 1024 * 16 // 8GB
			err = lvm.CreateSnapshot(img.VolumeGroup, img.LogicalVolume, lvName, lvSize)
			if err != nil {
				slog.Error("failed to create OS logical volume", "err", err, "vg", img.VolumeGroup, "lv", img.LogicalVolume)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}

			slog.Debug("OSボリュームののVGとLVでDBを更新", "Vol Key", volKey, "LV Name", lvName, "VG Name", img.VolumeGroup) // 取得したLV名をデータベースの登録
			vol := db.Volume{
				VolumeGroup:   &img.VolumeGroup,
				LogicalVolume: &lvName,
				Status:        func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
			}
			if err := m.Vc.UpdateVolume(volKey, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volKey", volKey)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}
			return volKey, nil

		case "data":
			dataVg := "vg2" // データボリューム用のボリュームグループ名を指定　現在は固定値
			slog.Info("LV Dataボリュームグループを使用", "vg", dataVg)

			lvName, err := m.Vc.Database.CreateDataLv(uint64(volSize), dataVg)
			if err != nil {
				slog.Error("failed to create Data logical volume", "err", err, "vg", dataVg, "size", volSize)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}
			slog.Debug("Dataボリュームの生成 成功", "LV Name", lvName, "VG Name", dataVg, "Size", volSize)

			// 取得したLV名とサイズで、データベースを更新
			vol := db.Volume{
				VolumeName:    &lvName, // 不要では？
				Status:        func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
				VolumeGroup:   &dataVg,
				LogicalVolume: &lvName,
				Size:          &volSize,
			}
			if err = m.Vc.UpdateVolume(volKey, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volKey", volKey)
				m.Vc.RollbackVolumeCreation(volKey)
				return "", err
			}
			slog.Debug("Dataボリュームの情報更新 成功", "volKey", volKey)
			return volKey, nil

		default:
			m.Vc.RollbackVolumeCreation(volKey)
			return "", errors.New("unsupported volume kind")
		}

	case "raw":
		m.Vc.RollbackVolumeCreation(volKey)
		return "", errors.New("unsupported volume type")
	default:
		m.Vc.RollbackVolumeCreation(volKey)
		return "", errors.New("unsupported volume type")
	}

	// データベースの状態を更新
	//return nil
}

// ボリューム削除,API, RemoveVolume(volId)
func (m *Marmot) RemoveVolume(volId string) error {
	// データベースからボリューム情報を取得
	vol, err := m.Vc.GetVolumeByKey(volId)
	if err != nil {
		return err
	}

	slog.Debug("RemoveVolume()", "volId", volId, "volType", *vol.Type, "volKind", *vol.Kind)

	// LV と qcow2ファイルの判断
	if *vol.Type == "lvm" {
		slog.Debug("Removing Logical volume", "volId", volId)

		// 物理的なボリュームの削除
		if err := lvm.RemoveLV(*vol.VolumeGroup, *vol.LogicalVolume); err != nil {
			slog.Error("lvm.RemoveLV()", "err", err)
		}
	} else if *vol.Type == "qcow2" {
		// qcow2ファイルの削除
		slog.Debug("Removing qcow2 file", "volId", volId, "qcow2Path", *vol.Path)

		// 物理的なボリュームの削除
		if err := qcow.RemoveQcow(*vol.Path); err != nil {
			slog.Error("qcow.RemoveQcow()", "err", err)
		}
	} else {
		slog.Error("Unknown volume type", "volId", volId)
		return errors.New("Unknown volume type")
	}

	// データベースからボリューム情報を削除
	if err := m.Vc.DeleteVolume(volId); err != nil {
		slog.Error("vc.DeleteVolume()", "err", err)
		return err
	}

	return nil
}

// ボリュームのリスト,API, GetVolumes(volType)
func (m *Marmot) GetOsVolumes() ([]db.Volume, error) {
	vols, err := m.Vc.ListVolumes("os")
	if err != nil {
		return nil, err
	}

	return vols, nil
}

func (m *Marmot) GetDataVolumes() ([]db.Volume, error) {
	vols, err := m.Vc.ListVolumes("data")
	if err != nil {
		return nil, err
	}

	return vols, nil
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

func (m *Marmot) ShowVolumeById(volumeId string) (*db.Volume, error) {
	vol, err := m.Vc.GetVolumeByKey("/" + volumeId)
	if err != nil {
		return nil, err
	}

	return &vol, nil
}

func (m *Marmot) UpdateVolumeById(volumeId string, volSpec api.Volume) (*db.Volume, error) {
	vol, err := m.Vc.GetVolumeByKey(volumeId)
	if err != nil {
		slog.Error("failed to get volume by key", "err", err, "volumeId", volumeId)
		return nil, err
	}

	slog.Debug("UpdateVolumeById()", "volumeId", volumeId, "volSpec Name", volSpec.Name)
	// 更新内容を構築 変更可能なのはサイズと名前のみ？
	if len(volSpec.Name) > 0 {
		vol.VolumeName = &volSpec.Name
	}
	util.Assign(&vol.Size, volSpec.Size)

	// データベースを更新
	if err := m.Vc.UpdateVolume(volumeId, vol); err != nil {
		return nil, err
	}

	return &vol, nil
}
