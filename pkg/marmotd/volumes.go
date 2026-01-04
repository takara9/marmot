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

func (m *Marmot) CreateNewVolume(v api.Volume) (*api.Volume, error) {
	slog.Debug("CreateVolume()", "name", v.Name, "type", *v.Type, "kind", *v.Kind)

	// 内容が設定されていない時はデフォルト値をセットする
	volName := util.OrDefault(v.Name, "vol1") // ボリューム名は必須 ボリューム名はラベルとして利用、ユニークである必要はない？
	volType := util.OrDefault(v.Type, "lvm")
	volKind := util.OrDefault(v.Kind, "os")
	volSize := util.OrDefault(v.Size, 0)
	volPath := util.OrDefault(v.Path, "") // パスはタイプと種類で決まるため、空で初期化

	// ボリュームの基本情報をデータベースに登録
	volSpec, err := m.Db.CreateVolumeOnDB(volName, volPath, volType, volKind, volSize)
	if err != nil {
		return nil, err
	}
	volId := volSpec.Id

	// ボリュームの実体を作成
	switch volType {
	case "qcow2":
		switch volKind {
		case "os":
			slog.Debug("qcow2 OSボリュームの生成", "volKind", volKind, "volId", volId)
			if v.OsVariant == nil {
				slog.Error("OsVariant is required for os volume")
				m.Db.RollbackVolumeCreation(volId)
				return nil, errors.New("OsVariant is required for os volume")
			}
			// OS名から該当するOSイメージテンプレートを取得
			img, err := m.Db.GetOsImgTempByOsVariant(*v.OsVariant)
			if err != nil {
				slog.Error("failed to get os image template", "err", err)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}
			if len(img.Qcow2Path) == 0 {
				slog.Error("os image template has no qcow2 path", "len(img.Qcow2Path)", len(img.Qcow2Path))
				m.Db.RollbackVolumeCreation(volId)
				return nil, errors.New("os image template has no qcow2 path")
			}

			slog.Debug("qcow2 OSボリュームをテンプレートから生成", "テンプレートパス", img.Qcow2Path)
			// テンプレートからOSボリュームを生成する
			seq, err := m.Db.GetSeqByKind("LVOS")
			if err != nil {
				return nil, err
			}
			qcow2Name := fmt.Sprintf("qcow2-%04d", seq)

			// パスを設定する場所を作るべきか？
			qcow2Path := fmt.Sprintf("/var/lib/marmot/volumes/%s.qcow2", qcow2Name)
			slog.Debug("新規qcow2ボリュームのパス決定", "qcow2Path", qcow2Path)

			err = qcow.CopyQcow(img.Qcow2Path, qcow2Path)
			if err != nil {
				slog.Error("failed to copy qcow2 image template", "err", err, "src", img.Qcow2Path, "dst", qcow2Path)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}

			vol := api.Volume{
				Status: func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
				Path:   &qcow2Path,
			}
			if err = m.Db.UpdateVolume(volId, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volId)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}

			return volSpec, nil
		case "data":
			slog.Debug("qcow2 Dataボリュームの生成", "volKind", volKind, "volId", volId)

			// データベースへの登録は済んでいるので、qcow2ファイルを新規作成
			seq, err := m.Db.GetSeqByKind("LVDATA")
			if err != nil {
				return nil, err
			}
			qcow2Name := fmt.Sprintf("qcow2-%04d", seq)
			qcow2Path := fmt.Sprintf("/var/lib/marmot/volumes/%s.qcow2", qcow2Name)
			slog.Debug("新規qcow2ボリュームのパス決定", "qcow2Path", qcow2Path)
			err = qcow.CreateQcow(qcow2Path, volSize)
			if err != nil {
				slog.Error("failed to create qcow2 data volume", "err", err, "path", qcow2Path)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}
			// 取得したqcow2パスで、データベースを更新
			// 取得したLV名とサイズで、データベースを更新
			vol := api.Volume{
				Status: func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
				Path:   &qcow2Path,
				Size:   &volSize,
			}
			if err = m.Db.UpdateVolume(volId, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volId)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}

			slog.Debug("Dataボリュームの情報更新 成功", "volId", volId)
			return volSpec, nil

		default:
			err := errors.New("unsupported unknown volume kind and type")
			m.Db.RollbackVolumeCreation(volId)
			return nil, err
		}
	case "lvm":
		switch volKind {
		case "os":
			slog.Debug("LV OSボリュームの生成", "volKind", volKind, "volId", volId)
			if v.OsVariant == nil {
				slog.Error("OsVariant is required for os volume")
				m.Db.RollbackVolumeCreation(volId)
				return nil, errors.New("OsVariant is required for os volume")
			}
			img, err := m.Db.GetOsImgTempByOsVariant(*v.OsVariant)
			if err != nil {
				slog.Error("failed to get os image template", "err", err, "os_variant", *v.OsVariant)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}

			slog.Debug("OSボリュームをスナップショットで生成", "volKind", volKind)
			seq, err := m.Db.GetSeqByKind("LVOS")
			if err != nil {
				return nil, err
			}
			lvName := fmt.Sprintf("oslv%04d", seq)
			var lvSize uint64 = 1024 * 1024 * 1024 * 16 // 8GB
			err = lvm.CreateSnapshot(img.VolumeGroup, img.LogicalVolume, lvName, lvSize)
			if err != nil {
				slog.Error("failed to create OS logical volume", "err", err, "vg", img.VolumeGroup, "lv", img.LogicalVolume)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}

			slog.Debug("OSボリュームののVGとLVでDBを更新", "Vol Id", volId, "LV Name", lvName, "VG Name", img.VolumeGroup) // 取得したLV名をデータベースの登録
			vol := api.Volume{
				VolumeGroup:   &img.VolumeGroup,
				LogicalVolume: &lvName,
				Status:        func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
			}
			if err := m.Db.UpdateVolume(volId, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volId)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}
			return volSpec, nil

		case "data":
			dataVg := "vg2" // データボリューム用のボリュームグループ名を指定　現在は固定値
			slog.Info("LV Dataボリュームグループを使用", "vg", dataVg)

			lvName, err := m.Db.CreateDataLv(uint64(volSize), dataVg)
			if err != nil {
				slog.Error("failed to create Data logical volume", "err", err, "vg", dataVg, "size", volSize)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}
			slog.Debug("Dataボリュームの生成 成功", "LV Name", lvName, "VG Name", dataVg, "Size", volSize)

			// 取得したLV名とサイズで、データベースを更新
			vol := api.Volume{
				Status:        func() *int { s := db.VOLUME_AVAILABLE; return &s }(),
				VolumeGroup:   &dataVg,
				LogicalVolume: &lvName,
				Size:          &volSize,
			}
			if err = m.Db.UpdateVolume(volId, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volId)
				m.Db.RollbackVolumeCreation(volId)
				return nil, err
			}
			slog.Debug("Dataボリュームの情報更新 成功", "volId", volId)
			return volSpec, nil

		default:
			m.Db.RollbackVolumeCreation(volId)
			return nil, errors.New("unsupported volume kind")
		}

	case "raw":
		m.Db.RollbackVolumeCreation(volId)
		return nil, errors.New("unsupported volume type")
	default:
		m.Db.RollbackVolumeCreation(volId)
		return nil, errors.New("unsupported volume type")
	}
}

// ボリューム削除,API, RemoveVolume(id)
func (m *Marmot) RemoveVolume(id string) error {
	// データベースからボリューム情報を取得
	vol, err := m.Db.GetVolumeById(id)
	if err != nil {
		return err
	}

	slog.Debug("RemoveVolume()", "volId", id, "volType", *vol.Type, "volKind", *vol.Kind)

	// LV と qcow2ファイルの判断
	if *vol.Type == "lvm" {
		slog.Debug("Removing Logical volume", "id", id)
		// 物理的なボリュームの削除
		if err := lvm.RemoveLV(*vol.VolumeGroup, *vol.LogicalVolume); err != nil {
			slog.Error("lvm.RemoveLV()", "err", err)
		}
	} else if *vol.Type == "qcow2" {
		// qcow2ファイルの削除
		slog.Debug("Removing qcow2 file", "id", id, "qcow2Path", *vol.Path)

		// 物理的なボリュームの削除
		if err := qcow.RemoveQcow(*vol.Path); err != nil {
			slog.Error("qcow.RemoveQcow()", "err", err)
		}
	} else {
		slog.Error("Unknown volume type", "id", id)
		return errors.New("Unknown volume type")
	}

	// データベースからボリューム情報を削除
	if err := m.Db.DeleteVolume(id); err != nil {
		slog.Error("vc.DeleteVolume()", "err", err)
		return err
	}

	return nil
}

// ボリュームのリスト,API, GetVolumes(volType)
func (m *Marmot) GetOsVolumes() ([]api.Volume, error) {
	vols, err := m.Db.ListVolumes("os")
	if err == db.ErrNotFound {
		return []api.Volume{}, nil
	} else if err != nil {
		return nil, err
	}

	return vols, nil
}

func (m *Marmot) GetDataVolumes() ([]api.Volume, error) {
	vols, err := m.Db.ListVolumes("data")
	if err == db.ErrNotFound {
		return []api.Volume{}, nil
	} else if err != nil {
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

func (m *Marmot) ShowVolumeById(id string) (*api.Volume, error) {
	vol, err := m.Db.GetVolumeById(id)
	if err != nil {
		return nil, err
	}

	return &vol, nil
}

func (m *Marmot) UpdateVolumeById(id string, volSpec api.Volume) (*api.Volume, error) {
	vol, err := m.Db.GetVolumeById(id)
	if err != nil {
		slog.Error("failed to get volume by key", "err", err, "volume id", id)
		return nil, err
	}

	slog.Debug("UpdateVolumeById()", "volumeId", id, "volSpec Name", volSpec.Name)
	util.Assign(&vol.Name, volSpec.Name)
	util.Assign(&vol.Size, volSpec.Size)

	// データベースを更新
	if err := m.Db.UpdateVolume(id, vol); err != nil {
		return nil, err
	}

	return &vol, nil
}
