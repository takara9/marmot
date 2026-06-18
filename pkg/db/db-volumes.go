package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	VOLUME_PENDING      = 0 // 待ち状態
	VOLUME_PROVISIONING = 1 // プロビジョニング中
	VOLUME_ERROR        = 2 // 問題発生
	VOLUME_AVAILABLE    = 3 // 利用可能
	VOLUME_DELETING     = 4 // 削除中
	VOLUME_UNAVAILABLE  = 5 // 実体欠損
	//VOLUME_DELETED      = 6 // 削除済み
)

var VolStatus = map[int]string{
	0: "PENDING",
	1: "PROVISIONING",
	2: "ERROR",
	3: "AVAILABLE",
	4: "DELETING",
	5: "UNAVAILABLE",
	//6: "DELETED",
}

// 仮想マシンを生成する時にボリュームを生成して、アタッチする
// OS or DATA ボリュームの作成
func (d *Database) CreateVolumeOnDB2(inputVol api.Volume) (*api.Volume, error) {
	slog.Debug("CreateVolume2()", "vol", inputVol)

	// ボリュームに名前があれば、その名前でロックする
	if strings.TrimSpace(inputVol.Metadata.Name) != "" {
		lockKey := "/lock/volume/" + strings.TrimSpace(inputVol.Metadata.Name)
		mutex, err := d.LockKey(lockKey)
		if err != nil {
			slog.Error("failed to lock", "err", err, "key", lockKey)
			return nil, err
		}
		defer d.UnlockKey(mutex)
	}

	//一意なIDを発行
	var id, uuidString, key string
	for {
		var tempVol api.Volume
		uuidString = uuid.New().String()
		id = uuidString[:5]
		key = VolumePrefix + "/" + id
		_, err := d.GetJSON(key, &tempVol)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("CreateVolumeOnDB()", "err", err)
			return nil, err
		}
	}

	// DeepCopyでinputVolをコピーする
	volume, err := util.DeepCopy(inputVol)
	if err != nil {
		slog.Error("failed to deep copy volume", "err", err)
		return nil, err
	}
	// ID、Key、Statusを設定する
	if volume.Status == nil {
		var status api.Status
		volume.Status = &status
	}

	api.SetVolumeID(&volume, id)
	volume.Metadata.Key = util.StringPtr(key)
	volume.Metadata.Uuid = util.StringPtr(uuidString)
	volume.Status.CreationTimeStamp = util.TimePtr(time.Now())
	volume.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	volume.Status.StatusCode = VOLUME_PENDING
	volume.Status.Status = util.StringPtr(VolStatus[volume.Status.StatusCode])

	// 指定が無い項目についてデフォルト値を設定する
	name := strings.TrimSpace(volume.Metadata.Name)
	if name == "" {
		volume.Metadata.Name = "vol-" + id
	} else {
		volume.Metadata.Name = name
	}
	// OSかDATAかの種別で、サイズのデフォルト値を変える
	if volume.Spec.Kind == nil || strings.TrimSpace(*volume.Spec.Kind) == "" {
		volume.Spec.Kind = util.StringPtr("data") // デフォルトはdata
		// サイズのデフォルト値を設定
		if volume.Spec.Size == nil {
			volume.Spec.Size = util.IntPtrInt(1) // 1GB
		}
	} else if strings.TrimSpace(*volume.Spec.Kind) == "os" {
		// OSボリュームはサイズ未指定(または0以下)時のみデフォルト値を設定
		if volume.Spec.Size == nil || *volume.Spec.Size <= 0 {
			volume.Spec.Size = util.IntPtrInt(16) // 16GB
		}
	}

	// ボリュームタイプのデフォルト値を設定し、パスを決定する
	if volume.Spec.Type == nil || strings.TrimSpace(*volume.Spec.Type) == "" {
		volume.Spec.Type = util.StringPtr("qcow2")
	}

	// QCOW2ボリュームの場合、パスを決定する。OSとDATAでパスを分ける。OSはboot-<id>.qcow2、DATAはdata-<id>.qcow2
	if strings.TrimSpace(*volume.Spec.Type) == "qcow2" {
		if strings.TrimSpace(*volume.Spec.Kind) == "os" {
			volume.Spec.Path = util.StringPtr(fmt.Sprintf("/var/lib/marmot/volumes/boot-%s.qcow2", api.VolumeID(volume)))
		} else {
			volume.Spec.Path = util.StringPtr(fmt.Sprintf("/var/lib/marmot/volumes/data-%s.qcow2", api.VolumeID(volume)))
		}
	}

	// LVMボリュームの場合、パスを決定する
	if strings.TrimSpace(*volume.Spec.Type) == "lvm" {
		configureLVMVolumeSpec(&volume.Spec, api.VolumeID(volume))
	}

	// OSボリュームの場合、OsVariantのデフォルト値を設定する  必要か？
	if strings.TrimSpace(*volume.Spec.Kind) == "os" {
		if volume.Spec.OsVariant == nil {
			volume.Spec.OsVariant = util.StringPtr("ubuntu22.04")
		}
	}

	byteData, _ := json.MarshalIndent(volume, "", "    ")
	debugPrintln("Volume to be created:", string(byteData))

	// データベースに登録
	if err := d.PutJSON(key, volume); err != nil {
		slog.Error("failed to write database data", "err", err, "key", *volume.Metadata.Key)
		return nil, err
	}
	return &volume, nil
}

func configureLVMVolumeSpec(spec *api.VolSpec, volumeID string) {
	if spec == nil {
		return
	}

	if spec.Kind != nil && *spec.Kind == "os" {
		volumeGroup := defaultOSVolumeGroup()
		spec.Path = util.StringPtr(fmt.Sprintf("/dev/%s/oslv-%s", volumeGroup, volumeID))
		spec.LogicalVolume = util.StringPtr(fmt.Sprintf("oslv-%s", volumeID))
		spec.VolumeGroup = util.StringPtr(volumeGroup)
		return
	}

	volumeGroup := defaultDataVolumeGroup()
	spec.Path = util.StringPtr(fmt.Sprintf("/dev/%s/datalv-%s", volumeGroup, volumeID))
	spec.LogicalVolume = util.StringPtr(fmt.Sprintf("datalv-%s", volumeID))
	spec.VolumeGroup = util.StringPtr(volumeGroup)
}

// ボリューム作成のロールバック
func (d *Database) RollbackVolumeCreation(id string) {
	slog.Debug("Rolling back volume creation", "id", id)

	lockKey := "/lock/volume/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return
	}
	defer d.UnlockKey(mutex)

	key := VolumePrefix + "/" + id
	if err := d.DeleteVolume(key); err != nil {
		slog.Error("failed to rollback volume creation", "err", err, "volKey", id)
	}
}

// / ボリュームを更新
func (d *Database) UpdateVolume(id string, updateData api.Volume) error {
	for {
		err := d.updateVolume(id, updateData)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateVolume() retrying due to update conflict", "volumeId", id)
			continue
		} else if err != nil {
			slog.Error("UpdateVolume()", "err", err)
			return err
		}
		break
	}

	//fmt.Println("=== 書き込みデータの情報確認 ===", "volume Id", id)
	//data, err := json.MarshalIndent(updateData, "", "  ")
	//if err != nil {
	//	slog.Error("json.MarshalIndent()", "err", err)
	//} else {
	//	fmt.Println("ボリューム情報(volume): ", string(data))
	//}

	return nil
}

// 内部関数 ボリュームを更新
func (d *Database) updateVolume(id string, updateData api.Volume) error {
	lockKey := "/lock/volume/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var rec api.Volume
	key := VolumePrefix + "/" + id
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return err
	}
	expected := resp.Kvs[0].ModRevision

	api.SetVolumeID(&rec, id)
	// パッチ適用
	util.PatchStruct(&rec, updateData)

	err = d.PutJSONCAS(key, expected, &rec)
	if err != nil {
		slog.Error("PutJSONCAS() failed", "err", err, "key", key, "expected", expected)
		return err
	}
	return nil
}

// データボリュームの削除
func (d *Database) DeleteVolume(id string) error {
	lockKey := "/lock/volume/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockkey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)
	key := VolumePrefix + "/" + id
	return d.DeleteJSON(key)
}

// データボリュームの一覧取得
func (d *Database) GetVolumes() ([]api.Volume, error) {
	var volumes []api.Volume
	var err error
	var resp *etcd.GetResponse

	resp, err = d.GetByPrefix(VolumePrefix)
	if err == ErrNotFound {
		slog.Debug("no volumes found", "key-prefix", VolumePrefix)
		return volumes, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", VolumePrefix)
		return volumes, err
	}
	for _, kv := range resp.Kvs {
		var vol api.Volume
		err := json.Unmarshal([]byte(kv.Value), &vol)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		volumes = append(volumes, vol)
	}

	return volumes, nil
}

// データボリュームの一覧取得
func (d *Database) ListVolumes(kind string) ([]api.Volume, error) {
	var volumes []api.Volume
	var err error
	var resp *etcd.GetResponse

	prefix := VolumePrefix
	resp, err = d.GetByPrefix(prefix)
	if err == ErrNotFound {
		slog.Debug("no volumes found", "key-prefix", prefix)
		return volumes, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", prefix)
		return volumes, err
	}
	for _, kv := range resp.Kvs {
		var vol api.Volume
		err := json.Unmarshal([]byte(kv.Value), &vol)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		if vol.Spec.Kind != nil && *vol.Spec.Kind == kind {
			volumes = append(volumes, vol)
		}
	}

	return volumes, nil
}

// データボリュームの情報取得
func (d *Database) GetVolumeById(id string) (api.Volume, error) {
	var vol api.Volume
	key := VolumePrefix + "/" + id
	slog.Debug("volume data", "key", key, "id", id)
	if _, err := d.GetJSON(key, &vol); err != nil {
		slog.Error("failed to get volume data", "err", err, "key", key, "id", id)
		return vol, err
	}
	return vol, nil
}

// データボリュームの一覧取得
func (d *Database) FindVolumeByName(name, kind string) ([]api.Volume, error) {
	var volumes []api.Volume
	prefix := VolumePrefix + "/"
	resp, err := d.GetByPrefix(prefix)
	if err != nil {
		if err == ErrNotFound {
			return volumes, nil
		}
		slog.Error("GetEtcdByPrefix() failed", "err", err, "key-prefix", prefix)
		return volumes, err
	}

	for _, kv := range resp.Kvs {
		var vol api.Volume
		err := json.Unmarshal([]byte(kv.Value), &vol)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		if vol.Metadata.Name == name {
			if vol.Spec.Kind != nil && *vol.Spec.Kind == kind {
				volumes = append(volumes, vol)
			}
		}
	}
	return volumes, nil
}

// OSボリュームの状態更新
// エラー時はログ出力して処理を中断する（panic は発生させない）
func (d *Database) UpdateVolumeStatus(id string, status int) {
	d.UpdateVolumeStatusMessage(id, status, "")
}

func (d *Database) UpdateVolumeStatusMessage(id string, status int, message string) {
	if len(id) == 0 {
		slog.Error("invalid volume id", "id", id)
		return
	}
	vol, err := d.GetVolumeById(id)
	if err != nil {
		slog.Error("UpdateVolumeStatusMessage() failed to get volume", "err", err, "id", id)
		return
	}
	vol.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	vol.Status.StatusCode = status
	vol.Status.Status = util.StringPtr(VolStatus[vol.Status.StatusCode])
	if len(message) > 0 {
		vol.Status.Message = util.StringPtr(message)
	} else {
		vol.Status.Message = nil
	}

	if err = d.UpdateVolume(id, vol); err != nil {
		slog.Error("UpdateVolumeStatusMessage() failed to update volume", "err", err, "volId", id)
	}
}

// 削除タイムスタンプのセット
func (d *Database) SetVolumeDeletionTimestamp(id string) {
	if len(id) == 0 {
		slog.Error("invalid volume id", "id", id)
		return
	}
	vol, err := d.GetVolumeById(id)
	if err != nil {
		slog.Error("SetVolumeDeletionTimestamp() failed to get volume", "err", err, "id", id)
		return
	}
	vol.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	if err = d.UpdateVolume(id, vol); err != nil {
		slog.Error("SetVolumeDeletionTimestamp() failed to update volume", "err", err, "volId", id)
	}
}

// 以下は未実装
// 仮想マシンからデータボリュームを作成
func createDataVolumefromVm() {

}

// アタッチデータボリューム ここではない、VMコントローラ側で実装するべきか？
func attachDataVolume() {

}

// デタッチデータボリューム　ここではない、VMコントローラ側で実装するべきか？
func detachDataVolume() {

}
