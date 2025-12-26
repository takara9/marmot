package db

import (
	"encoding/json"
	"errors"
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
	Database *Database
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

// ボリュームコントローラの生成
func NewVolumeController(url string) (*VolumeController, error) {
	var vc VolumeController
	var err error

	// データベース接続の生成
	vc.Database, err = NewDatabase(url)
	if err != nil {
		slog.Error("failed to create database", "err", err)
		return nil, err
	}
	return &vc, nil
}

func (vc *VolumeController) Close() error {
	return vc.Database.Close()
}

// 仮想マシンを生成する時にボリュームを生成して、アタッチする
// OS or DATA ボリュームの作成
func (vc *VolumeController) CreateVolumeOnDB(volName, volPath, volType, volKind string, volSize int) (*api.Volume, error) {
	slog.Debug("CreateVolume()", "volName", volName, "volPath", volPath, "volType", volType, "volKind", volKind, "volSize", volSize)
	var vol api.Volume
	vol.Id = uuid.New().String()
	switch volKind {
	case "data":
		vol.Key = util.StringPtr(VolumePrefix + "/" + vol.Id)
	case "os":
		vol.Key = util.StringPtr(OsImagePrefix + "/" + vol.Id)
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", volKind)
	}
	vol.Kind = util.StringPtr(volKind)
	vol.Type = util.StringPtr(volType)
	vol.Status = util.IntPtrInt(VOLUME_PROVISIONING)
	vol.Name = volName
	vol.Path = util.StringPtr(volPath)
	vol.Size = util.IntPtrInt(volSize)

	mutex, err := vc.Database.LockKey(*vol.Key)
	if err != nil {
		slog.Error("LockKey()", "err", err, "key", *vol.Key)
		return nil, err
	}
	defer vc.Database.UnlockKey(mutex)

	// データベースに登録
	if err := vc.Database.PutJSON(*vol.Key, vol); err != nil {
		slog.Error("PutJSON()", "err", err, "key", *vol.Key)
		return nil, err
	}

	return &vol, nil
}

func (vc *VolumeController) RollbackVolumeCreation(volKey string) {
	slog.Debug("Rolling back volume creation", "volKey", volKey)
	if err := vc.DeleteVolume(volKey); err != nil {
		slog.Error("failed to rollback volume creation", "err", err, "volKey", volKey)
	}
}

// ボリュームの情報更新
func (vc *VolumeController) UpdateVolume(key string, update api.Volume) error {
	mutex, err := vc.Database.LockKey(key)
	if err != nil {
		slog.Error("LockKey()", "err", err, "key", key)
		return err
	}
	defer vc.Database.UnlockKey(mutex)

	var rec api.Volume
	_, err = vc.Database.GetJSON(key, &rec)
	if err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return err
	}

	// 更新フィールドの反映
	if len(update.Name) > 0 {
		rec.Name = update.Name
	}
	util.Assign(&rec.Path, update.Path)
	util.Assign(&rec.Type, update.Type)
	util.Assign(&rec.Kind, update.Kind)
	util.Assign(&rec.Size, update.Size)
	util.Assign(&rec.Status, update.Status)
	util.Assign(&rec.VolumeGroup, update.VolumeGroup)
	util.Assign(&rec.LogicalVolume, update.LogicalVolume)
	util.Assign(&rec.OsName, update.OsName)
	util.Assign(&rec.OsVersion, update.OsVersion)

	// データベースに更新
	return vc.Database.PutJSON(key, rec)
}

// データボリュームの削除
func (vc *VolumeController) DeleteVolume(key string) error {
	lockKey := "/lock/volume/" + key
	mutex, err := vc.Database.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "key", lockKey)
		return err
	}
	defer vc.Database.UnlockKey(mutex)
	return vc.Database.DeleteJSON(key)
}

// データボリュームの一覧取得
func (vc *VolumeController) ListVolumes(kind string) ([]api.Volume, error) {
	var err error
	var resp *etcd.GetResponse
	switch kind {
	case "data":
		resp, err = vc.Database.GetByPrefix(VolumePrefix + "/")
	case "os":
		resp, err = vc.Database.GetByPrefix(OsImagePrefix + "/")
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", kind)
	}

	var volumes []api.Volume
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return volumes, nil
		}
		slog.Error("GetDataByPrefix() failed", "err", err, "key", VolumePrefix+"/")
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
func (vc *VolumeController) GetVolumeByKey(key string) (api.Volume, error) {
	var vol api.Volume
	_, err := vc.Database.GetJSON(key, &vol)
	if err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return vol, err
	}

	return vol, nil
}

// データボリュームの一覧取得
func (vc *VolumeController) FindVolumeByName(name, kind string) ([]api.Volume, error) {
	var key string
	switch kind {
	case "data":
		key = VolumePrefix + "/"
	case "os":
		key = OsImagePrefix + "/"
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", kind)
	}

	var volumes []api.Volume
	resp, err := vc.Database.GetByPrefix(key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return volumes, nil
		}
		slog.Error("GetByPrefix() failed", "err", err, "key", key)
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
