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
	VOLUME_INUSE        = 1 // 使用中
	VOLUME_AVAILABLE    = 2 // 利用可能
)

var VolStatus = map[int]string{
	0: "PROVISIONING",
	1: "INUSE",
	2: "AVAILABLE",
}

// 仮想マシンを生成する時にボリュームを生成して、アタッチする
// OS or DATA ボリュームの作成
func (d *Database) CreateVolumeOnDB(volName, volPath, volType, volKind string, volSize int) (*api.Volume, error) {
	slog.Debug("CreateVolume()", "volName", volName, "volPath", volPath, "volType", volType, "volKind", volKind, "volSize", volSize)
	var vol api.Volume

	lockKey := "/lock/volume/" + volName
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return &vol, err
	}
	defer d.UnlockKey(mutex)

	vol.Id = uuid.New().String()
	key := VolumePrefix + "/" + vol.Id // DATAボリューム
	vol.Key = util.StringPtr(key)
	vol.Kind = util.StringPtr(volKind)
	vol.Type = util.StringPtr(volType)
	vol.Status = util.IntPtrInt(VOLUME_PROVISIONING)
	vol.Name = util.StringPtr(volName)
	vol.Path = util.StringPtr(volPath)
	vol.Size = util.IntPtrInt(volSize)

	if err := d.PutJSON(key, vol); err != nil {
		slog.Error("failed to write database data", "err", err, "key", *vol.Key)
		return nil, err
	}
	return &vol, nil
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

	// 更新フィールドの反映
	util.Assign(&rec.Name, updateData.Name)
	util.Assign(&rec.Path, updateData.Path)
	util.Assign(&rec.Type, updateData.Type)
	util.Assign(&rec.Kind, updateData.Kind)
	util.Assign(&rec.Size, updateData.Size)
	util.Assign(&rec.Status, updateData.Status)
	util.Assign(&rec.VolumeGroup, updateData.VolumeGroup)
	util.Assign(&rec.LogicalVolume, updateData.LogicalVolume)
	util.Assign(&rec.OsName, updateData.OsName)
	util.Assign(&rec.OsVersion, updateData.OsVersion)

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
		err := json.Unmarshal([]byte(kv.Value), &vol)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		if *vol.Kind == kind {
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
		if *vol.Kind == kind {
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
	var lvSize uint64 = 1024 * 1024 * 1024 * 16 // 8GB
	err = lvm.CreateSnapshot(tempVg, tempLv, lvName, lvSize)
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

	// 論理ボリュームを作成
	lvName := fmt.Sprintf("data%04d", seq)
	lvSize := 1024 * 1024 * 1024 * sz
	err = lvm.CreateLV(vg, lvName, lvSize)
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
