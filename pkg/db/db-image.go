package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

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

func (d *Database) getUniqueImageID() (string, error) {
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
			slog.Error("getUniqueImageID()", "err", err)
			return "", err
		}
	}
	return id, nil
}

// URLのイメージをダウンロードして、それからイメージを作成する
func (d *Database) MakeImageEntryFromURL(name, url string) (string, error) {
	slog.Debug("MakeImageEntryFromURL() called", "name", name, "url", url)

	//一意なIDを発行
	id, err := d.getUniqueImageID()
	if err != nil {
		slog.Error("MakeImageEntryFromURL()", "err", err)
		return "", err
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
			StatusCode:          IMAGE_PENDING,
			Status:              util.StringPtr(ImageStatus[IMAGE_PENDING]),
			CreationTimeStamp:   util.TimePtr(time.Now()),
			LastUpdateTimeStamp: util.TimePtr(time.Now()),
			Message:             util.StringPtr("イメージの作成処理の開始待ち"),
		},
	}
	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, img); err != nil {
		slog.Error("MakeImageEntryFromURL()", "err", err)
		return "", err
	}

	return id, nil
}

// ブートボリュームからイメージを作成する
func (d *Database) MakeImageEntryFromRunningVM(serverId, name string) (api.Image, error) {
	slog.Debug("MakeImageEntryFromRunningVM() called", "name", name, "serverId", serverId)

	//一意なIDを発行
	id, err := d.getUniqueImageID()
	if err != nil {
		slog.Error("MakeImageEntryFromRunningVM()", "err", err)
		return api.Image{}, err
	}

	server, err := d.GetServerById(serverId)
	if err != nil {
		slog.Error("MakeImageEntryFromRunningVM() failed to get server by id", "err", err, "serverId", serverId)
		return api.Image{}, err
	}
	bootVol := server.Spec.BootVolume

	// ボリュームがOSボリュームであることを確認
	if *bootVol.Spec.Kind != "os" {
		slog.Error("MakeImageEntryFromRunningVM() volume is not an OS volume", "volumeId", bootVol.Id)
		return api.Image{}, fmt.Errorf("volume with id %v is not an OS volume", bootVol.Id)
	}

	//イメージの基本情報を保存
	labels := map[string]interface{}{
		"source":   "bootVolume",
		"serverId": serverId,
	}
	var img api.Image
	if bootVol.Spec.Type != nil && *bootVol.Spec.Type == "qcow2" {
		// イメージを書き込むディレクトリの作成
		imageDir := fmt.Sprintf("/var/lib/marmot/images/%s", id)
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			slog.Error("CreateImageFromVolume() failed to create image directory", "err", err, "imageDir", imageDir)
			return api.Image{}, err
		}
		// イメージのqcow2ボリューム名を設定
		imagePath := fmt.Sprintf("%s/osimage-%s.qcow2", imageDir, id)
		img = api.Image{
			Id: id,
			Metadata: &api.Metadata{
				Name:   &name,
				Labels: &labels,
			},
			Spec: &api.ImageSpec{
				Kind:          bootVol.Spec.Kind, // ポインタの値を直接使用
				Type:          bootVol.Spec.Type,
				VolumeGroup:   nil,
				LogicalVolume: nil,
				LvPath:        nil,
				Qcow2Path:     util.StringPtr(imagePath),
				Size:          bootVol.Spec.Size,
				SourceUrl:     nil,
			},
			Status: &api.Status{
				StatusCode:          IMAGE_PENDING,
				Status:              util.StringPtr(ImageStatus[IMAGE_PENDING]),
				CreationTimeStamp:   util.TimePtr(time.Now()),
				LastUpdateTimeStamp: util.TimePtr(time.Now()),
			},
		}
	} else if bootVol.Spec.Type != nil && *bootVol.Spec.Type == "lvm" {
		// イメージの論理ボリューム名を設定
		logicalVolumePath := fmt.Sprintf("/dev/%s/osimage-%s", *bootVol.Spec.VolumeGroup, id)
		logicalVolumeName := fmt.Sprintf("osimage-%s", id)
		img = api.Image{
			Id: id,
			Metadata: &api.Metadata{
				Name:   &name,
				Labels: &labels,
			},
			Spec: &api.ImageSpec{
				Kind:          bootVol.Spec.Kind, // ポインタの値を直接使用
				Type:          bootVol.Spec.Type,
				VolumeGroup:   bootVol.Spec.VolumeGroup,
				LogicalVolume: util.StringPtr(logicalVolumeName),
				LvPath:        util.StringPtr(logicalVolumePath),
				Qcow2Path:     nil,
				Size:          bootVol.Spec.Size,
				SourceUrl:     nil,
			},
			Status: &api.Status{
				StatusCode: IMAGE_PENDING,
				Status:     util.StringPtr(ImageStatus[IMAGE_PENDING]),
			},
		}
	} else {
		slog.Error("MakeImageEntryFromRunningVM() unsupported volume type", "volumeId", bootVol.Id, "type", bootVol.Spec.Type)
		return api.Image{}, fmt.Errorf("unsupported volume type for volume with id %v: %v", bootVol.Id, bootVol.Spec.Type)
	}

	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, img); err != nil {
		slog.Error("MakeImageEntryFromRunningVM()", "err", err)
		return api.Image{}, err
	}

	return img, nil
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
		slog.Debug("no images found", "key-prefix", ImagePrefix)
		return images, nil
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", ImagePrefix)
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
func (d *Database) UpdateImageStatus(id string, status int) {
	slog.Debug("SetImageStatus() called", "id", id, "status", status)
	image, err := d.GetImage(id)
	if err != nil {
		slog.Error("SetDeleteTimestamp() GetImage() failed", "err", err, "imageId", id)
		panic(err)
	}
	image.Status.StatusCode = status
	image.Status.Status = util.StringPtr(ImageStatus[status])
	image.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if err := d.UpdateImage(id, image); err != nil {
		slog.Error("SetDeleteTimestamp() UpdateImage() failed", "err", err, "imageId", id)
		panic(err)
	}
}

// イメージのオブジェクトを部分更新
func (d *Database) UpdateImage(id string, spec api.Image) error {
	for {
		err := d.updateImage(id, spec)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateImage() retrying due to update conflict", "imageId", id)
			continue
		} else if err != nil {
			slog.Error("UpdateImage()", "err", err)
			return err
		}
		break
	}

	fmt.Println("=== 書き込みデータの情報確認 ===", "image Id", id)
	data3, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		fmt.Println("イメージ情報(image): ", string(data3))
	}

	return nil
}

// 内部イメージを更新
func (d *Database) updateImage(id string, spec api.Image) error {
	lockKey := "/lock/image/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("failed to lock", "err", err, "lockKey", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	var rec api.Image
	key := ImagePrefix + "/" + id
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		slog.Error("GetJSON() failed", "err", err, "key", key)
		return err
	}
	expected := resp.Kvs[0].ModRevision

	rec.Id = id
	// パッチ適用
	util.PatchStruct(&rec, spec)

	err = d.PutJSONCAS(key, expected, &rec)
	if err != nil {
		slog.Error("PutJSONCAS() failed", "err", err, "key", key, "expected", expected)
		return err
	}
	return nil
}

// イメージの削除予定日時をセットする
func (d *Database) SetDeleteTimestampImage(id string) error {
	image, err := d.GetImage(id)
	if err != nil {
		slog.Error("SetDeleteTimestamp() GetImage() failed", "err", err, "imageId", id)
		return err
	}
	image.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	if err := d.UpdateImage(id, image); err != nil {
		slog.Error("SetDeleteTimestamp() UpdateImage() failed", "err", err, "imageId", id)
		return err
	}
	return nil
}

// イメージの全リストを返す
func (d *Database) FindImageByName(name string) (api.Image, error) {
	slog.Debug("FindImageByName() called", "name", name)
	var err error
	var resp *etcd.GetResponse

	resp, err = d.GetByPrefix(ImagePrefix)
	if err == ErrNotFound {
		slog.Debug("no images found", "key-prefix", ImagePrefix)
		return api.Image{}, err
	} else if err != nil {
		slog.Error("GetByPrefix() failed", "err", err, "key-prefix", ImagePrefix)
		return api.Image{}, err
	}

	for _, kv := range resp.Kvs {
		var img api.Image
		err := json.Unmarshal(kv.Value, &img)
		if err != nil {
			slog.Error("FindImageByName() failed to unmarshal image", "err", err)
			continue
		}
		if img.Metadata != nil && img.Metadata.Name != nil && *img.Metadata.Name == name {
			return img, nil
		}
	}

	return api.Image{}, fmt.Errorf("image not found with name: %v", name)
}
