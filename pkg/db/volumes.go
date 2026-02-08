package db

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	VOLUME_PROVISIONING = 0 // プロビジョニング中
	VOLUME_ERROR        = 1 // 問題発生
	VOLUME_AVAILABLE    = 2 // 利用可能
)

var VolStatus = map[int]string{
	0: "PROVISIONING",
	1: "ERROR",
	2: "AVAILABLE",
}

// 仮想マシンを生成する時にボリュームを生成して、アタッチする
// OS or DATA ボリュームの作成
func (d *Database) CreateVolumeOnDB(volName, volPath, volType, volKind string, volSize int) (*api.Volume, error) {
	slog.Debug("CreateVolume()", "volName", volName, "volPath", volPath, "volType", volType, "volKind", volKind, "volSize", volSize)
	var vol api.Volume
	var meta api.Metadata
	vol.Metadata = &meta
	var spec api.VolSpec
	vol.Spec = &spec
	var Status api.Status
	vol.Status2 = &Status

	lockKey := "/lock/volume/" + volName
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return &vol, err
	}
	defer d.UnlockKey(mutex)

	//一意なIDを発行
	var key string
	for {
		vol.Id = uuid.New().String()[:7]
		key = VolumePrefix + "/" + vol.Id
		_, err := d.GetJSON(key, &vol)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("CreateVolumeOnDB()", "err", err)
			return nil, err
		}
	}

	vol.Metadata.Key = util.StringPtr(key)
	vol.Metadata.Name = util.StringPtr(volName)
	vol.Status2.Status = util.IntPtrInt(VOLUME_PROVISIONING)
	vol.Spec.Kind = util.StringPtr(volKind)
	vol.Spec.Type = util.StringPtr(volType)
	vol.Spec.Path = util.StringPtr(volPath)
	vol.Spec.Size = util.IntPtrInt(volSize)
	if err := d.PutJSON(key, vol); err != nil {
		slog.Error("failed to write database data", "err", err, "key", *vol.Metadata.Key)
		return nil, err
	}
	return &vol, nil
}

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
	if volume.Status2 == nil {
		var status api.Status
		volume.Status2 = &status
	}

	volume.Id = id
	volume.Metadata.Key = util.StringPtr(key)
	volume.Status2.Status = util.IntPtrInt(VOLUME_PROVISIONING)

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
			volume.Spec.Path = util.StringPtr(fmt.Sprintf("/dev/%s/oslv%s", "vg1", volume.Id))
			volume.Spec.LogicalVolume = util.StringPtr(fmt.Sprintf("oslv%s", volume.Id))
			volume.Spec.VolumeGroup = util.StringPtr("vg1")
		} else {
			volume.Spec.Path = util.StringPtr(fmt.Sprintf("/dev/%s/datalv%s", "vg2", volume.Id))
			volume.Spec.LogicalVolume = util.StringPtr(fmt.Sprintf("datalv%s", volume.Id))
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
		vol.Status2 = &Status

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
		if *vol.Metadata.Name == name && *vol.Spec.Kind == kind {
			volumes = append(volumes, vol)
		}
	}

	return volumes, nil
}

// OSテンプVolのスナップショットを作成してデバイス名を返す
// この関数が呼ばれているのは、以下の一箇所のみ
// https://github.com/takara9/marmot/blob/main/pkg/marmotd/vm-create.go#L60
func (d *Database) CreateOsLv(tempVg string, tempLv string) (string, error) {
	seq, err := d.GetSeqByKind("LVOS")
	if err != nil {
		return "", err
	}

	// スナップショットで、OS用論理ボリュームを作成
	lvName := fmt.Sprintf("oslv%04d", seq)
	err = lvm.CreateSnapshot(tempVg, tempLv, lvName, 16)
	if err != nil {
		return "", err
	}
	return lvName, err
}

// データボリュームの作成
// この関数が呼ばれているのは、以下の一箇所のみ
// https://github.com/takara9/marmot/blob/main/pkg/marmotd/vm-create.go#L97
func (d *Database) CreateDataLv(sz uint64, vg string) (string, error) {
	seq, err := d.GetSeqByKind("LVDATA")
	if err != nil {
		return "", err
	}

	// 論理ボリュームを作成 MB単位でサイズ指定
	lvName := fmt.Sprintf("data%04d", seq)
	err = lvm.CreateLV(vg, lvName, sz)
	if err != nil {
		return "", err
	}
	return lvName, err
}

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
// 仮想マシンからイメージテンプレートを作成
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
