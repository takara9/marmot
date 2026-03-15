package db

import (
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	IMAGE_PENDING         = 0 // 待ち状態
	IMAGE_CREATING        = 1 // プロビジョニング中
	IMAGE_CREATION_FAILED = 2 // 問題発生
	IMAGE_AVAILABLE       = 3 // 利用可能
	IMAGE_DELETING        = 4 // 削除中
	IMAGE_DELETED         = 5 // 削除済み
)

var ImageStatus = map[int]string{
	0: "PENDING",
	1: "CREATING",
	2: "FAILED",
	3: "AVAILABLE",
	4: "DELETING",
	5: "DELETED",
}

// URLのイメージをダウンロードして、それからイメージを作成する
func (d *Database) CreateImageFromURL(name, url string) (string, error) {
	slog.Debug("CreateImageFromURL() called", "name", name, "url", url)

	//一意なIDを発行
	var id string
	var key string
	for {
		var tempVol api.Image
		id = uuid.New().String()[:5]
		key = ImagePrefix + "/" + id
		_, err := d.GetJSON(key, &tempVol)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("CreateImageFromURL()", "err", err)
			return "", err
		}
	}
	//イメージの基本情報を保存
	img := api.Image{
		Id: id,
		Metadata: &api.Metadata{
			Name: &name,
		},
		Spec: &api.ImageSpec{
			SourceUrl: &url,
		},
		Status: &api.Status{
			Status: util.IntPtrInt(IMAGE_PENDING),
		},
	}
	err := d.PutJSON(key, img)
	if err != nil {
		slog.Error("CreateImageFromURL()", "err", err)
		return "", err
	}

	return id, nil
}

// IDを指定してイメージの情報を取得する
func (d *Database) GetImage(id string) (api.Image, error) {
	slog.Debug("GetImage() called", "id", id)
	key := ImagePrefix + "/" + id
	var img api.Image
	_, err := d.GetJSON(key, &img)
	if err != nil {
		slog.Error("GetImage()", "err", err)
		return api.Image{}, err
	}

	return img, nil
}

// イメージの全リストを返す
func (d *Database) GetImages() ([]api.Image, error) {
	slog.Debug("GetImages() called", "", "")
	var images []api.Image
	var err error
	var resp *etcd.GetResponse

	resp, err = d.GetByPrefix(ImagePrefix)
	if err == ErrNotFound {
		slog.Debug("no volumes found", "key-prefix", VolumePrefix)
		return images, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", VolumePrefix)
		return images, err
	}

	for _, kv := range resp.Kvs {
		var img api.Image
		err := json.Unmarshal(kv.Value, &img)
		if err != nil {
			slog.Error("GetImages() failed to unmarshal image", "err", err)
			continue
		}
		images = append(images, img)
	}

	return images, nil
}

// IDで特定してイメージを削除する
func (d *Database) DeleteImage(id string) error {
	slog.Debug("DeleteImages() called", "id", id)
	key := ImagePrefix + "/" + id
	return d.DeleteJSON(key)
}

// ステータスの変更
func (d *Database) SetImageStatus(id string, status int) {
	slog.Debug("SetImageStatus() called", "id", id, "status", status)
	key := ImagePrefix + "/" + id
	var img api.Image
	_, err := d.GetJSON(key, &img)
	if err != nil {
		slog.Error("SetImageStatus() failed to get image", "err", err)
		return
	}
	img.Status.Status = util.IntPtrInt(status)
	err = d.PutJSON(key, img)
	if err != nil {
		slog.Error("SetImageStatus() failed to update image status", "err", err)
		return
	}
}

func (d *Database) UpdateImage(id string, imageSpec api.Image) error {
	slog.Debug("UpdateImageById() called", "id", id)
	key := ImagePrefix + "/" + id
	var img api.Image
	_, err := d.GetJSON(key, &img)
	if err != nil {
		slog.Error("UpdateImageById() failed to get image", "err", err)
		return err
	}

	// 変更可能なフィールドを更新する
	if imageSpec.Metadata != nil && imageSpec.Metadata.Name != nil {
		img.Metadata.Name = imageSpec.Metadata.Name
	}
	if imageSpec.Spec != nil && imageSpec.Spec.SourceUrl != nil {
		img.Spec.SourceUrl = imageSpec.Spec.SourceUrl
	}

	err = d.PutJSON(key, img)
	if err != nil {
		slog.Error("UpdateImageById() failed to update image", "err", err)
		return err
	}
	return nil
}