package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	VOLUME_DELETED      = 5 // 削除済み
)

var VolStatus = map[int]string{
	0: "PENDING",
	1: "PROVISIONING",
	2: "ERROR",
	3: "AVAILABLE",
	4: "DELETING",
	5: "DELETED",
}

// 仮想マシンを生成する時にボリュームを生成して、アタッチする
// OS or DATA ボリュームの作成
func (d *Database) CreateVolumeOnDB2(inputVol api.Volume) (*api.Volume, error) {
	slog.Debug("CreateVolume2()", "vol", inputVol)

	// ボリュームに名前があれば、その名前でロックする
	if inputVol.Metadata.Name != nil {
		lockKey := "/lock/volume/" + *inputVol.Metadata.Name
		mutex, err := d.LockKey(lockKey)
		if err != nil {
			slog.Error("failed to lock", "err", err, "key", lockKey)
			return nil, err
		}
		defer d.UnlockKey(mutex)
	}

	//一意なIDを発行
	var id string
	var key string
	for {
		var tempVol api.Volume
		id = uuid.New().String()[:5]
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
	if volume.Metadata == nil {
		var metadata api.Metadata
		volume.Metadata = &metadata
	}
	if volume.Spec == nil {
		var spec api.VolSpec
		volume.Spec = &spec
	}
	if volume.Status == nil {
		var status api.Status
		volume.Status = &status
	}

	volume.Id = id
	volume.Metadata.Key = util.StringPtr(key)
	volume.Status.CreationTimeStamp = util.TimePtr(time.Now())
	volume.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	volume.Status.Status = util.IntPtrInt(VOLUME_PROVISIONING)

	// 指定が無い項目についてデフォルト値を設定する
	if volume.Metadata.Name == nil {
		volume.Metadata.Name = util.StringPtr("vol-" + id)
	}
	// OSかDATAかの種別で、サイズのデフォルト値を変える
	if volume.Spec.Kind == nil {
		volume.Spec.Kind = util.StringPtr("data") // デフォルトはdata
		// サイズのデフォルト値を設定
		if volume.Spec.Size == nil {
			volume.Spec.Size = util.IntPtrInt(1) // 1GB
		}
	} else if *volume.Spec.Kind == "os" {
		// OSボリュームのサイズのデフォルト値を設定
		volume.Spec.Size = util.IntPtrInt(16) // 16GB
	}

	// ボリュームタイプのデフォルト値を設定し、パスを決定する
	if volume.Spec.Type == nil {
		volume.Spec.Type = util.StringPtr("qcow2")
	}
	if *volume.Spec.Type == "qcow2" {
		volume.Spec.Path = util.StringPtr(fmt.Sprintf("/var/lib/marmot/volumes/%s.qcow2", volume.Id))
	}

	// LVMボリュームの場合、パスを決定する
	if *volume.Spec.Type == "lvm" {
		if *volume.Spec.Kind == "os" {
			volume.Spec.Path = util.StringPtr(fmt.Sprintf("/dev/%s/oslv-%s", "vg1", volume.Id))
			volume.Spec.LogicalVolume = util.StringPtr(fmt.Sprintf("oslv-%s", volume.Id))
			volume.Spec.VolumeGroup = util.StringPtr("vg1")
		} else {
			volume.Spec.Path = util.StringPtr(fmt.Sprintf("/dev/%s/datalv-%s", "vg2", volume.Id))
			volume.Spec.LogicalVolume = util.StringPtr(fmt.Sprintf("datalv-%s", volume.Id))
			volume.Spec.VolumeGroup = util.StringPtr("vg2")
		}
	}

	// OSボリュームの場合、OsVariantのデフォルト値を設定する
	if *volume.Spec.Kind == "os" {
		if volume.Spec.OsVariant == nil {
			volume.Spec.OsVariant = util.StringPtr("ubuntu22.04")
		}
	}

	byteData, _ := json.MarshalIndent(volume, "", "    ")
	fmt.Println("Volume to be created:", string(byteData))

	// データベースに登録
	if err := d.PutJSON(key, volume); err != nil {
		slog.Error("failed to write database data", "err", err, "key", *volume.Metadata.Key)
		return nil, err
	}
	return &volume, nil
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

// ボリュームの情報更新 CASを持ちるべき？
func (d *Database) UpdateVolume(id string, updateData api.Volume) error {
	lockKey := "/lock/volume/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockkey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var rec api.Volume
	key := VolumePrefix + "/" + id
	if _, err := d.GetJSON(key, &rec); err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return err
	}

	// デバッグ用ログ出力
	debugData1, _ := json.MarshalIndent(updateData, "", "    ")
	fmt.Println("Updating data\n", string(debugData1))

	// 更新フィールドの反映  rec <- updateData
	util.PatchStruct(&rec, &updateData)

	// デバッグ用ログ出力
	debugData2, _ := json.MarshalIndent(rec, "", "    ")
	fmt.Println("Updated volume data\n", string(debugData2))

	// データベースに更新
	return d.PutJSON(key, rec)

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
		var volSpec api.VolSpec
		vol.Spec = &volSpec
		var Metadata api.Metadata
		vol.Metadata = &Metadata
		var Status api.Status
		vol.Status = &Status

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
		var volSpec api.VolSpec
		vol.Spec = &volSpec
		var Metadata api.Metadata
		vol.Metadata = &Metadata
		var Status api.Status
		vol.Status = &Status

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
		if vol.Metadata.Name != nil && *vol.Metadata.Name == name {
			if vol.Spec.Kind != nil && *vol.Spec.Kind == kind {
				volumes = append(volumes, vol)
			}
		}
	}
	return volumes, nil
}

// OSボリュームの状態更新
// エラーが出る場合、パニックして停止させるべき
func (d *Database) UpdateVolumeStatus(id string, status int) {
	vol, err := d.GetVolumeById(id)
	if err != nil {
		slog.Error("failed to get volume by id", "err", err, "id", id)
		panic(fmt.Sprintf("failed to get volume by id: %s", id))
	}
	vol.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	vol.Status.Status = util.IntPtrInt(status)
	if err = d.UpdateVolume(id, vol); err != nil {
		slog.Error("failed to update volume", "err", err, "volId", id)
		panic(fmt.Sprintf("failed to update volume: %s", id))
	}
}

// 削除タイムスタンプのセット
func (d *Database) SetVolumeDeletionTimestamp(id string) {
	vol, err := d.GetVolumeById(id)
	if err != nil {
		slog.Error("failed to get volume by id", "err", err, "id", id)
		panic(fmt.Sprintf("failed to get volume by id: %s", id))
	}
	vol.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	if err = d.UpdateVolume(id, vol); err != nil {
		slog.Error("failed to update volume", "err", err, "volId", id)
		panic(fmt.Sprintf("failed to update volume: %s", id))
	}
}


// 以下は未実装

// イメージテンプレートの登録
func registerImageTemplate() {

}

// イメージテンプレートの削除
func deleteImageTemplate() {

}

// イメージテンプレートの一覧取得
func listImageTemplates() {

}

// イメージテンプレートの情報取得
func createImageTemplatefromVm() {

}

// 仮想マシンからデータボリュームを作成
func createDataVolumefromVm() {

}

// アタッチデータボリューム ここではない、VMコントローラ側で実装するべきか？
func attachDataVolume() {

}

// デタッチデータボリューム　ここではない、VMコントローラ側で実装するべきか？
func detachDataVolume() {

}
