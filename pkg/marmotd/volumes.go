package marmotd

// ボリュームの情報管理の関数群

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/qcow"
	"github.com/takara9/marmot/pkg/util"
)

func resolveImageTemplateByVolumeNode(m *Marmot, volSpec api.Volume) (api.Image, error) {
	osVariant := ""
	if volSpec.Spec != nil && volSpec.Spec.OsVariant != nil {
		osVariant = strings.TrimSpace(*volSpec.Spec.OsVariant)
	}
	if osVariant == "" {
		return api.Image{}, errors.New("osVariant is required for os volume")
	}

	targetNode := ""
	if volSpec.Metadata != nil && volSpec.Metadata.NodeName != nil {
		targetNode = strings.TrimSpace(*volSpec.Metadata.NodeName)
	}

	if targetNode != "" {
		return m.Db.FindImageByNameAndNode(osVariant, targetNode)
	}

	return m.Db.FindImageByName(osVariant)
}

func requestedOSVolumeSizeGB(volSpec api.Volume, img api.Image) (int, error) {
	const defaultOSSizeGB = 16

	requestedSize := defaultOSSizeGB
	if volSpec.Spec != nil && volSpec.Spec.Size != nil && *volSpec.Spec.Size > 0 {
		requestedSize = *volSpec.Spec.Size
	}

	imageSize := defaultOSSizeGB
	if img.Spec != nil && img.Spec.Size != nil && *img.Spec.Size > 0 {
		imageSize = *img.Spec.Size
	}

	if requestedSize < imageSize {
		return 0, fmt.Errorf("requested os volume size %dGB is smaller than image size %dGB", requestedSize, imageSize)
	}

	return requestedSize, nil
}

func (m *Marmot) CreateNewVolume(id string) (*api.Volume, error) {
	volSpec, err := m.Db.GetVolumeById(id)
	if err != nil {
		slog.Error("failed to get volume by id after creation", "err", err, "volId", id)
		return nil, err
	}

	slog.Debug("Creating volume", "volId", volSpec.Id, "volType", *volSpec.Spec.Type, "volKind", *volSpec.Spec.Kind)

	// ボリュームの実体を作成
	switch *volSpec.Spec.Type {
	case "qcow2":
		switch *volSpec.Spec.Kind {
		case "os":
			slog.Debug("qcow2 OSボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id, "osVariant", volSpec.Spec.OsVariant)
			if volSpec.Spec.OsVariant == nil {
				slog.Error("osVariant is nil in volume spec", "volId", volSpec.Id)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, errors.New("osVariant is required for os volume")
			}
			img, err := resolveImageTemplateByVolumeNode(m, volSpec)
			if err != nil {
				slog.Error("failed to get os image template", "err", err, "osVariant", *volSpec.Spec.OsVariant)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, err
			}

			requestedSizeGB, err := requestedOSVolumeSizeGB(volSpec, img)
			if err != nil {
				slog.Error("invalid requested os volume size", "err", err, "volId", volSpec.Id)
				m.Db.UpdateVolumeStatusMessage(volSpec.Id, db.VOLUME_ERROR, err.Error())
				return nil, err
			}
			volSpec.Spec.Size = util.IntPtrInt(requestedSizeGB)

			imageSizeGB := 16
			if img.Spec != nil && img.Spec.Size != nil && *img.Spec.Size > 0 {
				imageSizeGB = *img.Spec.Size
			}

			//if img.Spec.Qcow2Path == nil || len(*img.Spec.Qcow2Path) == 0 {
			if img.Spec.Qcow2Path == nil {
				slog.Error("os image template has no qcow2 path", "", 0)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, errors.New("os image template has no qcow2 path")
			}

			slog.Debug("qcow2 OSボリュームをテンプレートから生成", "テンプレートパス", *img.Spec.Qcow2Path)
			err = qcow.CopyQcow(*img.Spec.Qcow2Path, *volSpec.Spec.Path)
			if err != nil {
				slog.Error("failed to copy qcow2 image template", "err", err, "src", *img.Spec.Qcow2Path, "dst", *volSpec.Spec.Path)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, err
			}

			if requestedSizeGB > imageSizeGB {
				err = qcow.ResizeQcow(*volSpec.Spec.Path, requestedSizeGB)
				if err != nil {
					slog.Error("failed to resize qcow2 os volume", "err", err, "path", *volSpec.Spec.Path, "sizeGB", requestedSizeGB)
					m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
					return nil, err
				}
			}

			// 取得したLV名とサイズで、データベースを更新
			slog.Debug("qcow2ボリュームの状態変更", "volId", volSpec.Id)
			volSpec.Status.Message = nil
			volSpec.Status.StatusCode = db.VOLUME_AVAILABLE
			volSpec.Status.Status = util.StringPtr(db.VolStatus[db.VOLUME_AVAILABLE])
			volSpec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
			m.Db.UpdateVolumeStatus(volSpec.Id, volSpec.Status.StatusCode)
			slog.Debug("qcow2ボリュームの更新完了", "volId", volSpec.Id)

			return &volSpec, nil
		case "data":
			slog.Debug("qcow2 Dataボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id)
			err = qcow.CreateQcow(*volSpec.Spec.Path, *volSpec.Spec.Size)
			if err != nil {
				slog.Error("failed to create qcow2 data volume", "err", err, "path", *volSpec.Spec.Path)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, err
			}
			slog.Debug("Dataボリュームの情報更新 成功", "volId", volSpec.Id)
			volSpec.Status.Message = nil
			volSpec.Status.StatusCode = db.VOLUME_AVAILABLE
			volSpec.Status.Status = util.StringPtr(db.VolStatus[db.VOLUME_AVAILABLE])
			volSpec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
			m.Db.UpdateVolumeStatus(volSpec.Id, volSpec.Status.StatusCode)
			return &volSpec, nil
		default:
			err := errors.New("unsupported unknown volume kind and type")
			m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
			return nil, err
		}
	case "lvm":
		switch *volSpec.Spec.Kind {
		case "os":
			slog.Debug("LV OSボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id)

			slog.Debug("qcow2 OSボリュームの生成", "volKind", *volSpec.Spec.Kind, "volId", volSpec.Id, "osVariant", volSpec.Spec.OsVariant)
			if volSpec.Spec.OsVariant == nil {
				slog.Error("osVariant is nil in volume spec", "volId", volSpec.Id)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, errors.New("osVariant is required for os volume")
			}
			img, err := resolveImageTemplateByVolumeNode(m, volSpec)
			if err != nil {
				slog.Error("failed to get os image template", "err", err, "osVariant", *volSpec.Spec.OsVariant)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, err
			}

			slog.Debug("OSボリュームをスナップショットで生成", "volKind", *volSpec.Spec.Kind)
			size := uint64(4 * 1024 * 1024 * 1024) // スナップショットサイズは4GB固定
			err = lvm.CreateSnapshot(*img.Spec.VolumeGroup, *img.Spec.LogicalVolume, *volSpec.Spec.LogicalVolume, size)
			if err != nil {
				slog.Error("failed to create OS logical volume", "err", err, "vg", *img.Spec.VolumeGroup, "lv", *img.Spec.LogicalVolume)
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, err
			}

			slog.Debug("OSボリュームののVGとLVでDBを更新", "Vol Id", volSpec.Id) // 取得したLV名をデータベースの登録
			volSpec.Status.Message = nil
			volSpec.Status.StatusCode = db.VOLUME_AVAILABLE
			volSpec.Status.Status = util.StringPtr(db.VolStatus[db.VOLUME_AVAILABLE])
			volSpec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
			m.Db.UpdateVolumeStatus(volSpec.Id, volSpec.Status.StatusCode)
			slog.Debug("OSボリュームの情報更新 成功", "volId", volSpec.Id)
			return &volSpec, nil

		case "data":
			lvSize := uint64(*volSpec.Spec.Size) * 1024 * 1024 * 1024
			err = lvm.CreateLV(*volSpec.Spec.VolumeGroup, *volSpec.Spec.LogicalVolume, lvSize)
			if err != nil {
				m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
				return nil, err
			}

			slog.Debug("Dataボリュームの生成 成功", "LV Name", *volSpec.Spec.LogicalVolume, "VG Name", *volSpec.Spec.VolumeGroup, "Size", *volSpec.Spec.Size, "volId", volSpec.Id)
			volSpec.Status.Message = nil
			volSpec.Status.StatusCode = db.VOLUME_AVAILABLE
			volSpec.Status.Status = util.StringPtr(db.VolStatus[db.VOLUME_AVAILABLE])
			volSpec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
			m.Db.UpdateVolumeStatus(volSpec.Id, volSpec.Status.StatusCode)
			slog.Debug("Dataボリュームの情報更新 成功", "volId", volSpec.Id)
			return &volSpec, nil
		default:
			m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
			return nil, errors.New("unsupported volume kind")
		}
	case "raw":
		m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
		return nil, errors.New("unsupported volume type")
	default:
		m.Db.UpdateVolumeStatus(volSpec.Id, db.VOLUME_ERROR)
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
				vol.Status.Message = util.StringPtr(fmt.Sprintf("failed to remove logical volume: %v", err))
				// ここでエラーが発生して、err = "Logical volume vg1/oslv-76c5f is used by another device."　の場合
				// これは、論理ボリュームがまだ仮想マシンにアタッチされたままになっていることが原因で発生するエラーです。
				// そのため、論理ボリュームを削除する前に、仮想マシンからデタッチする必要があります。
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

	return nil
}

// 全てのボリュームを取得する関数,API, GetVolumes()
func (m *Marmot) GetVolumes() ([]api.Volume, error) {
	vols, err := m.Db.GetVolumes()
	if err == db.ErrNotFound {
		return []api.Volume{}, nil
	} else if err != nil {
		return nil, err
	}

	return vols, nil
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

// データボリュームの一覧取得,API, GetDataVolumes()
func (m *Marmot) GetDataVolumes() ([]api.Volume, error) {
	vols, err := m.Db.ListVolumes("data")
	if err == db.ErrNotFound {
		return []api.Volume{}, nil
	} else if err != nil {
		return nil, err
	}

	return vols, nil
}

// IDでボリュームの情報を取得する関数,API, GetVolumeById(volId)
func (m *Marmot) GetVolumeById(id string) (*api.Volume, error) {
	vol, err := m.Db.GetVolumeById(id)
	if err != nil {
		return nil, err
	}

	return &vol, nil
}

// IDでボリュームの情報を更新する関数,API, UpdateVolumeById(volId, volSpec)
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

func CheckVolumeBackingStore(volume api.Volume) error {
	if volume.Spec == nil {
		return nil
	}

	backingPath, backingType := getVolumeBackingStore(volume.Spec)
	if backingPath == "" {
		return nil
	}

	if _, err := os.Stat(backingPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing volume backing store: %s %s", backingType, backingPath)
		}
		return fmt.Errorf("failed to inspect %s %s: %w", backingType, backingPath, err)
	}

	return nil
}

func getVolumeBackingStore(spec *api.VolSpec) (string, string) {
	if spec == nil {
		return "", ""
	}

	volType := strings.TrimSpace(util.OrDefault(spec.Type, ""))
	switch volType {
	case "qcow2":
		path := strings.TrimSpace(util.OrDefault(spec.Path, ""))
		if path == "" {
			return "", ""
		}
		return path, "qcow2 file"
	case "lvm":
		path := strings.TrimSpace(util.OrDefault(spec.Path, ""))
		if path != "" {
			return path, "logical volume"
		}

		volumeGroup := strings.TrimSpace(util.OrDefault(spec.VolumeGroup, ""))
		logicalVolume := strings.TrimSpace(util.OrDefault(spec.LogicalVolume, ""))
		if volumeGroup == "" || logicalVolume == "" {
			return "", ""
		}

		return filepath.Join("/dev", volumeGroup, logicalVolume), "logical volume"
	default:
		return "", ""
	}
}

// 以下は未実装

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
