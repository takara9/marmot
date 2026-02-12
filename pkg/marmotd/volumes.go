package marmotd

// ボリュームの情報管理の関数群

import (
	"errors"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/qcow"
	"github.com/takara9/marmot/pkg/util"
)

func (m *Marmot) CreateNewVolume(id string) (*api.Volume, error) {
	volSpec, err := m.Db.GetVolumeById(id)
	if err != nil {
		slog.Error("failed to get volume by id after creation", "err", err, "volId", id)
		return nil, err
	}

	slog.Info("Creating volume", "volId", volSpec.Id, "volType", *volSpec.Spec.Type, "volKind", *volSpec.Spec.Kind)

	// ボリュームの実体を作成
	switch *volSpec.Spec.Type {
	case "qcow2":
		switch *volSpec.Spec.Kind {
		case "os":
			slog.Debug("qcow2 OSボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id, "osVariant", volSpec.Spec.OsVariant)
			img, err := m.Db.GetOsImgTempByOsVariant(*volSpec.Spec.OsVariant)
			if err != nil {
				slog.Error("failed to get os image template", "err", err)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}
			if len(img.Qcow2Path) == 0 {
				slog.Error("os image template has no qcow2 path", "len(img.Qcow2Path)", len(img.Qcow2Path))
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, errors.New("os image template has no qcow2 path")
			}

			slog.Debug("qcow2 OSボリュームをテンプレートから生成", "テンプレートパス", img.Qcow2Path)
			err = qcow.CopyQcow(img.Qcow2Path, *volSpec.Spec.Path)
			if err != nil {
				slog.Error("failed to copy qcow2 image template", "err", err, "src", img.Qcow2Path, "dst", *volSpec.Spec.Path)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}

			// 取得したLV名とサイズで、データベースを更新
			slog.Debug("qcow2ボリュームの状態変更", "volId", volSpec.Id)
			vol := api.Volume{
				Status: &api.Status{
					Status:              util.IntPtrInt(db.VOLUME_AVAILABLE),
					LastUpdateTimeStamp: util.TimePtr(time.Now()),
					CreationTimeStamp:   util.TimePtr(time.Now()),
				},
			}
			if err = m.Db.UpdateVolume(volSpec.Id, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volSpec.Id)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}
			slog.Debug("qcow2ボリュームの更新完了", "volId", volSpec.Id)

			return &volSpec, nil
		case "data":
			slog.Debug("qcow2 Dataボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id)
			err = qcow.CreateQcow(*volSpec.Spec.Path, *volSpec.Spec.Size)
			if err != nil {
				slog.Error("failed to create qcow2 data volume", "err", err, "path", *volSpec.Spec.Path)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}
			slog.Debug("Dataボリュームの情報更新 成功", "volId", volSpec.Id)

			// 状態を更新
			vol := api.Volume{
				Status: &api.Status{
					CreationTimeStamp:   util.TimePtr(time.Now()),
					LastUpdateTimeStamp: util.TimePtr(time.Now()),
					Status:              util.IntPtrInt(db.VOLUME_AVAILABLE),
				},
			}
			if err = m.Db.UpdateVolume(volSpec.Id, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volSpec.Id)
				return nil, err
			}
			return &volSpec, nil

		default:
			err := errors.New("unsupported unknown volume kind and type")
			m.Db.RollbackVolumeCreation(volSpec.Id)
			return nil, err
		}
	case "lvm":
		switch *volSpec.Spec.Kind {
		case "os":
			slog.Debug("LV OSボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id)
			img, err := m.Db.GetOsImgTempByOsVariant(*volSpec.Spec.OsVariant)
			if err != nil {
				slog.Error("failed to get os image template", "err", err, "os_variant", *volSpec.Spec.OsVariant)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}

			slog.Debug("OSボリュームをスナップショットで生成", "volKind", *volSpec.Spec.Kind)
			size := uint64(4 * 1024 * 1024 * 1024) // スナップショットサイズは4GB固定
			err = lvm.CreateSnapshot(img.VolumeGroup, img.LogicalVolume, *volSpec.Spec.LogicalVolume, size)
			if err != nil {
				slog.Error("failed to create OS logical volume", "err", err, "vg", img.VolumeGroup, "lv", img.LogicalVolume)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}

			slog.Debug("OSボリュームののVGとLVでDBを更新", "Vol Id", volSpec.Id) // 取得したLV名をデータベースの登録
			vol := api.Volume{
				Status: &api.Status{
					CreationTimeStamp:   util.TimePtr(time.Now()),
					LastUpdateTimeStamp: util.TimePtr(time.Now()),
					Status:              util.IntPtrInt(db.VOLUME_AVAILABLE),
				},
			}
			if err := m.Db.UpdateVolume(volSpec.Id, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volSpec.Id)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}

			slog.Debug("OSボリュームの情報更新 成功", "volId", volSpec.Id)
			return &volSpec, nil

		case "data":
			lvSize := uint64(*volSpec.Spec.Size) * 1024 * 1024 * 1024

			err = lvm.CreateLV(*volSpec.Spec.VolumeGroup, *volSpec.Spec.LogicalVolume, lvSize)
			if err != nil {
				return nil, err
			}

			slog.Debug("Dataボリュームの生成 成功", "LV Name", *volSpec.Spec.LogicalVolume, "VG Name", *volSpec.Spec.VolumeGroup, "Size", *volSpec.Spec.Size, "volId", volSpec.Id)

			// 取得したLV名とサイズで、データベースを更新
			vol := api.Volume{
				Status: &api.Status{
					LastUpdateTimeStamp: util.TimePtr(time.Now()),
					CreationTimeStamp:   util.TimePtr(time.Now()),
					Status:              util.IntPtrInt(db.VOLUME_AVAILABLE),
				},
			}
			if err = m.Db.UpdateVolume(volSpec.Id, vol); err != nil {
				slog.Error("failed to update volume", "err", err, "volId", volSpec.Id)
				m.Db.RollbackVolumeCreation(volSpec.Id)
				return nil, err
			}
			slog.Debug("Dataボリュームの情報更新 成功", "volId", volSpec.Id)

			return &volSpec, nil

		default:
			m.Db.RollbackVolumeCreation(volSpec.Id)
			return nil, errors.New("unsupported volume kind")
		}

	case "raw":
		m.Db.RollbackVolumeCreation(volSpec.Id)
		return nil, errors.New("unsupported volume type")
	default:
		m.Db.RollbackVolumeCreation(volSpec.Id)
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

	slog.Debug("RemoveVolume()", "volId", id, "volType", *vol.Spec.Type, "volKind", *vol.Spec.Kind)

	// LV と qcow2ファイルの判断
	if *vol.Spec.Type == "lvm" {
		slog.Debug("Removing Logical volume", "id", id)
		// 物理的なボリュームの削除
		if vol.Spec.VolumeGroup != nil && vol.Spec.LogicalVolume != nil {
			if err := lvm.RemoveLV(*vol.Spec.VolumeGroup, *vol.Spec.LogicalVolume); err != nil {
				slog.Error("lvm.RemoveLV()", "err", err)
			}
		}
	} else if *vol.Spec.Type == "qcow2" {
		// qcow2ファイルの削除
		if vol.Spec.Path != nil {
			// 物理的なボリュームの削除
			if err := qcow.RemoveQcow(*vol.Spec.Path); err != nil {
				slog.Error("qcow.RemoveQcow()", "err", err)
			}
		}
	} else {
		// 未知のタイプの場合データベースからのみ削除する
		slog.Error("Unknown volume type", "id", id)
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

func (m *Marmot) GetVolumes() ([]api.Volume, error) {
	vols, err := m.Db.GetVolumes()
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

func (m *Marmot) GetVolumeById(id string) (*api.Volume, error) {
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

	util.PatchStruct(&vol, &volSpec)
	vol.Id = id

	// データベースを更新
	if err := m.Db.UpdateVolume(id, vol); err != nil {
		return nil, err
	}

	return &vol, nil
}
