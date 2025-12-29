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

/*
ボリュームの管理用データ構造体
対象は、OSイメージテンプレートとデータボリューム
*/
type VolumeController struct {
	Db *Database
}

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

	var key string
	vol.Id = uuid.New().String()
	switch volKind {
	case "data":
		key = VolumePrefix + "/" + vol.Id
	case "os":
		key = OsImagePrefix + "/" + vol.Id
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", volKind)
	}

	vol.Key = util.StringPtr(key)
	vol.Kind = util.StringPtr(volKind)
	vol.Type = util.StringPtr(volType)
	vol.Status = util.IntPtrInt(VOLUME_PROVISIONING)
	vol.Name = volName
	vol.Path = util.StringPtr(volPath)
	vol.Size = util.IntPtrInt(volSize)

	if err := d.PutJSON(*vol.Key, vol); err != nil {
		slog.Error("failed to write database data", "err", err, "key", *vol.Key)
		return nil, err
	}
	return &vol, nil
}

// ボリューム作成のロールバック
func (d *Database) RollbackVolumeCreation(volKey string) {
	slog.Debug("Rolling back volume creation", "volKey", volKey)

	lockKey := "/lock/volume/" + volKey
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return
	}
	defer d.UnlockKey(mutex)

	if err := d.DeleteVolume(volKey); err != nil {
		slog.Error("failed to rollback volume creation", "err", err, "volKey", volKey)
	}
}

// ボリュームの情報更新
func (d *Database) UpdateVolume(volKey string, updateData api.Volume) error {
	lockKey := "/lock/volume/" + volKey
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockkey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)


	var rec api.Volume
	if _, err := d.GetJSON(volKey, &rec); err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", volKey)
		return err
	}

	// 更新フィールドの反映
	if len(updateData.Name) > 0 {
		rec.Name = updateData.Name
	}
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
	return d.PutJSON(volKey, rec)
}

// データボリュームの削除
func (d *Database) DeleteVolume(volKey string) error {
	lockKey := "/lock/volume/" + volKey
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockkey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	return d.DeleteJSON(volKey)
}

// データボリュームの一覧取得
func (d *Database) ListVolumes(kind string) ([]api.Volume, error) {
	var volumes []api.Volume
	var err error
	var resp *etcd.GetResponse

	switch kind {
	case "data":
		resp, err = d.GetByPrefix(VolumePrefix + "/")
	case "os":
		resp, err = d.GetByPrefix(OsImagePrefix + "/")
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", kind)
	}

	if err != nil {
		slog.Error("GetEtcdByPrefix() failed", "err", err, "key", VolumePrefix+"/")
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

// データボリュームの情報取得
func (d *Database) GetVolumeByKey(key string) (api.Volume, error) {
	var vol api.Volume
	if _, err := d.GetJSON(key, &vol); err != nil {
		slog.Error("failed to get volume data", "err", err, "key", key)
		return vol, err
	}
	return vol, nil
}

// データボリュームの一覧取得
func (d *Database) FindVolumeByName(name, kind string) ([]api.Volume, error) {
	var volumes []api.Volume
	var key string

	switch kind {
	case "data":
		key = VolumePrefix + "/"
	case "os":
		key = OsImagePrefix + "/"
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", kind)
	}

	resp, err := d.GetByPrefix(key)
	if err != nil {
		slog.Error("GetEtcdByPrefix() failed", "err", err, "key", key)
		return volumes, err
	}

	for _, kv := range resp.Kvs {
		var vol api.Volume
		err := json.Unmarshal([]byte(kv.Value), &vol)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		if vol.Name != name {
			continue
		}
		volumes = append(volumes, vol)
	}

	return volumes, nil
}

// OSテンプVolのスナップショットを作成してデバイス名を返す
// この関数が呼ばれているのは、以下の一箇所のみ
// https://github.com/takara9/marmot/blob/main/pkg/marmotd/vm-create.go#L60
func (d *Database) CreateOsLv(tempVg string, tempLv string) (string, error) {
	// シリアルを取得
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
	// シリアルを取得
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
