package db

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
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
	Kind       string // ボリュームの種類  os, data
	Type       string // ボリュームのタイプ qcow2, lvm, raw
	Id         string // UUID
	Key        string // etcdに登録したキー
	VolumeName string // ボリューム名
	SizeGB     int    // サイズ(GB)
	Path       string // ボリュームの保存パス
	Status     int    // 状態 (int: VOLUME_INUSE=1, VOLUME_AVAILABLE=2)
	OsName     string // OS名 (osボリュームの場合)
	OsVersion  string // OSバージョン (osボリュームの場合)
}

const (
	VOLUME_INUSE     = 1 // 使用中
	VOLUME_AVAILABLE = 2 // 利用可能
)

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

// 仮想マシンを生成する時にボリュームを生成して、アタッチする

// OS or DATA ボリュームの作成
func (vc *VolumeController) CreateVolume(volName, path, volType, kind string, SizeGB int) (string, error) {
	var vol Volume

	// パラメータチェック
	if volName == "" || path == "" || volType == "" || kind == "" || SizeGB <= 0 {
		return "", fmt.Errorf("invalid parameters")
	}
	
	// ボリューム情報の生成
	vol.Id = uuid.New().String()
	switch kind {
	case "data":
		vol.Key = VolumePrefix + "/" + vol.Id
	case "os":
		vol.Key = OsImagePrefix + "/" + vol.Id
	default:
		return vol.Id, fmt.Errorf("unknown volume kind: %s", kind)
	}
	vol.Kind = kind
	vol.Type = volType
	vol.Status = VOLUME_AVAILABLE
	vol.VolumeName = volName
	vol.Path = path
	vol.SizeGB = SizeGB

	// データベースに登録
	err := vc.Database.PutDataEtcd(vol.Key, vol)
	if err != nil {
		return vol.Id, err
	}
	vc.vol = append(vc.vol, vol)
	return vol.Id, nil
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
		if vol.VolumeName != name {
			continue
		}
		volumes = append(volumes, vol)
	}

	return volumes, nil
}


// OSボリュームの作成
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

	vol.OsName = osName
	vol.OsVersion = osVersion

	vc.Database.Lock.Lock()
	defer vc.Database.Lock.Unlock()
	if err := vc.Database.PutDataEtcd(vol.Key, vol); err != nil {
		return "", err
	}

	return key, nil
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
