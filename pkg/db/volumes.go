package db

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
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
	vol      []Volume
}

type Volume struct {
	Kind          *string   // ボリュームの種類  os, data
	Type          *string   // ボリュームのタイプ qcow2, lvm, raw
	Id            uuid.UUID // UUID
	Key           *string   // etcdに登録したキー
	VolumeName    *string   // ボリューム名
	VolumeGroup   *string   // ボリュームグループ (lvm形式の場合)
	LogicalVolume *string   // 論理ボリューム名 (lvm形式の場合)
	Size          *int      // サイズ(MB)
	Path          *string   // ボリュームの保存パス
	Status        *int      // 状態 (int: VOLUME_INUSE=1, VOLUME_AVAILABLE=2)
	OsName        *string   // OS名 (osボリュームの場合)
	OsVersion     *string   // OSバージョン (osボリュームの場合)
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
func (vc *VolumeController) CreateVolumeOnDB(volName, volPath, volType, volKind string, volSize int) (string, error) {
	slog.Debug("CreateVolume()", "volName", volName, "volPath", volPath, "volType", volType, "volKind", volKind, "volSize", volSize)
	var vol Volume

	// パラメータチェック
	//if volName == "" || volPath == "" || volType == "" || volKind == "" || volSize <= 0 {
	//	return "", fmt.Errorf("invalid parameters")
	//}

	// ボリューム情報の生成
	vol.Id = uuid.New()

	switch volKind {
	case "data":
		vol.Key = util.StringPtr(VolumePrefix + "/" + vol.Id.String())
	case "os":
		vol.Key = util.StringPtr(OsImagePrefix + "/" + vol.Id.String())
	default:
		return "", fmt.Errorf("unknown volume kind: %s", volKind)
	}
	vol.Kind = util.StringPtr(volKind)
	vol.Type = util.StringPtr(volType)
	vol.Status = util.IntPtrInt(VOLUME_PROVISIONING)
	vol.VolumeName = util.StringPtr(volName)
	vol.Path = util.StringPtr(volPath)
	vol.Size = util.IntPtrInt(volSize)
	// データベースに登録
	err := vc.Database.PutDataEtcd(*vol.Key, vol)
	if err != nil {
		slog.Error("PutDataEtcd() failed", "err", err, "key", *vol.Key)
		return "", err
	}
	vc.vol = append(vc.vol, vol) //なんのため？要らんのでは？
	return *vol.Key, nil
}

func (vc *VolumeController) RollbackVolumeCreation(volKey string) {
	slog.Debug("Rolling back volume creation", "volKey", volKey)
	if err := vc.DeleteVolume(volKey); err != nil {
		slog.Error("failed to rollback volume creation", "err", err, "volKey", volKey)
	}
}

// ボリュームの情報更新
func (vc *VolumeController) UpdateVolume(key string, vol Volume) error {
	vc.Database.Lock.Lock()
	defer vc.Database.Lock.Unlock()

	//if vol.Key == nil {
	//	return fmt.Errorf("volume key is nil")
	//}

	resp, err := vc.Database.GetByKey(key)
	if err != nil {
		slog.Error("GetByKey() failed", "err", err, "key", key)
		return err
	}

	var updateVol Volume
	err = json.Unmarshal([]byte(resp), &updateVol)
	if err != nil {
		slog.Error("Unmarshal() failed", "err", err, "key", key)
		return err
	}

	// 更新フィールドの反映
	util.Assign(&updateVol.VolumeName, vol.VolumeName)
	util.Assign(&updateVol.Path, vol.Path)
	util.Assign(&updateVol.Type, vol.Type)
	util.Assign(&updateVol.Kind, vol.Kind)
	util.Assign(&updateVol.Size, vol.Size)
	util.Assign(&updateVol.Status, vol.Status)
	util.Assign(&updateVol.VolumeGroup, vol.VolumeGroup)
	util.Assign(&updateVol.LogicalVolume, vol.LogicalVolume)
	util.Assign(&updateVol.OsName, vol.OsName)
	util.Assign(&updateVol.OsVersion, vol.OsVersion)

	// データベースに更新
	return vc.Database.PutDataEtcd(key, updateVol)
}

// データボリュームの削除
func (vc *VolumeController) DeleteVolume(key string) error {
	return vc.Database.DelByKey(key)
}

// データボリュームの一覧取得
func (vc *VolumeController) ListVolumes(kind string) ([]Volume, error) {
	var volumes []Volume
	var err error
	var resp *etcd.GetResponse

	switch kind {
	case "data":
		resp, err = vc.Database.GetEtcdByPrefix(VolumePrefix + "/")
	case "os":
		resp, err = vc.Database.GetEtcdByPrefix(OsImagePrefix + "/")
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", kind)
	}

	if err != nil {
		slog.Error("GetEtcdByPrefix() failed", "err", err, "key", VolumePrefix+"/")
		return volumes, err
	}
	for _, kv := range resp.Kvs {
		var vol Volume
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
func (vc *VolumeController) GetVolumeByKey(key string) (Volume, error) {
	var vol Volume
	resp, err := vc.Database.GetByKey(key)
	if err != nil {
		slog.Error("GetEtcdByKey() failed", "err", err, "key", key)
		return vol, err
	}
	err = json.Unmarshal([]byte(resp), &vol)
	if err != nil {
		slog.Error("Unmarshal() failed", "err", err, "key", key)
		return vol, err
	}
	return vol, nil
}

// データボリュームの一覧取得
func (vc *VolumeController) FindVolumeByName(name, kind string) ([]Volume, error) {
	var volumes []Volume
	var key string

	switch kind {
	case "data":
		key = VolumePrefix + "/"
	case "os":
		key = OsImagePrefix + "/"
	default:
		return nil, fmt.Errorf("unknown volume kind: %s", kind)
	}

	resp, err := vc.Database.GetEtcdByPrefix(key)
	if err != nil {
		slog.Error("GetEtcdByPrefix() failed", "err", err, "key", key)
		return volumes, err
	}

	for _, kv := range resp.Kvs {
		var vol Volume
		err := json.Unmarshal([]byte(kv.Value), &vol)
		if err != nil {
			slog.Error("Unmarshal() failed", "err", err, "key", string(kv.Key))
			continue
		}
		if *vol.VolumeName != name {
			continue
		}
		volumes = append(volumes, vol)
	}

	return volumes, nil
}

// OSボリュームの作成
/*
func (vc *VolumeController) CreateOSVolume(volName, path, volType string, sizeGB int, osName, osVersion string) (string, error) {
	if osName == "" || osVersion == "" {
		return "", fmt.Errorf("invalid parameters")
	}

	key, err := vc.CreateVolume(volName, path, volType, "os", sizeGB)
	if err != nil {
		return "", err
	}
	vol, err := vc.GetVolumeByKey(key)
	if err != nil {
		return "", err
	}

	vol.OsName = &osName
	vol.OsVersion = &osVersion

	vc.Database.Lock.Lock()
	defer vc.Database.Lock.Unlock()
	if err := vc.Database.PutDataEtcd(*vol.Key, vol); err != nil {
		return "", err
	}

	return key, nil
}
*/

/*
	TODO: データベースの操作とボリュームの実体作成を行う関数群を分割する
*/
//
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
