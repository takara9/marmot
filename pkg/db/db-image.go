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
	IMAGE_PENDING         = 0 // 待ち状態
	IMAGE_CREATING        = 1 // プロビジョニング中
	IMAGE_CREATION_FAILED = 2 // 問題発生
	IMAGE_AVAILABLE       = 3 // 利用可能
	IMAGE_DELETING        = 4 // 削除中
	IMAGE_DELETED         = 5 // 削除済み
	IMAGE_WAITING         = 6 // ヘッドノードの作成完了待ち

	// Image label keys for distributed sync
	ImageLabelSyncRole     = "syncRole"
	ImageLabelHeadImageID  = "headImageId"
	ImageLabelHeadNodeName = "headNodeName"
	ImageLabelSource       = "source"
	ImageLabelServerID     = "serverId"
)

var ImageStatus = map[int]string{
	0: "PENDING",
	1: "CREATING",
	2: "FAILED",
	3: "AVAILABLE",
	4: "DELETING",
	5: "DELETED",
	6: "WAITING",
}

// ==================== Image Label Helpers ====================
// Follower イメージの sync 情報を etcd オブジェクトに格納するための
// 型安全なヘルパー関数。これらはラベルに直接作用します。

// SetFollowerSyncLabels は follower イメージの sync 情報をラベルに設定する
func SetFollowerSyncLabels(labels map[string]interface{}, syncRole, headImageID, headNodeName string) {
	if labels == nil {
		return
	}
	labels[ImageLabelSyncRole] = syncRole
	labels[ImageLabelHeadImageID] = strings.TrimSpace(headImageID)
	if node := strings.TrimSpace(headNodeName); node != "" {
		labels[ImageLabelHeadNodeName] = node
	}
}

// SetImageSourceLabels は image source 情報をラベルに設定する
func SetImageSourceLabels(labels map[string]interface{}, source string, serverID string) {
	if labels == nil {
		return
	}
	if src := strings.TrimSpace(source); src != "" {
		labels[ImageLabelSource] = src
	}
	if sID := strings.TrimSpace(serverID); sID != "" {
		labels[ImageLabelServerID] = sID
	}
}

// GetFollowerSyncRole はラベルから syncRole を取得する
func GetFollowerSyncRole(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[ImageLabelSyncRole].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

// GetHeadImageID はラベルから headImageId を取得する
func GetHeadImageID(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[ImageLabelHeadImageID].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

// GetHeadNodeName はラベルから headNodeName を取得する
func GetHeadNodeName(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[ImageLabelHeadNodeName].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

// GetImageSource はラベルから source を取得する
func GetImageSource(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[ImageLabelSource].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

// GetImageServerID はラベルから serverId を取得する
func GetImageServerID(labels map[string]interface{}) string {
	if labels == nil {
		return ""
	}
	val, ok := labels[ImageLabelServerID].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func (d *Database) getUniqueImageID() (string, string, error) {
	var id string
	var uuidString string
	var key string
	for {
		var tempVol api.Image
		uuidString = uuid.New().String()
		id = uuidString[:5]
		key = ImagePrefix + "/" + id
		_, err := d.GetJSON(key, &tempVol)
		if err == ErrNotFound {
			break
		} else if err != nil {
			slog.Error("getUniqueImageID()", "err", err)
			return "", "", err
		}
	}
	return id, uuidString, nil
}

// URLのイメージをダウンロードして、それからイメージを作成する
func (d *Database) MakeImageEntryFromURL(name, url string) (string, error) {
	return "", fmt.Errorf("nodeName is required: use MakeImageEntryFromURLWithNode")
}

// APIのImage定義をそのまま受け取り、イメージを作成する。
func (d *Database) MakeImageEntryFromSpec(imageSpec api.Image) (string, error) {
	if strings.TrimSpace(imageSpec.Metadata.Name) == "" {
		return "", fmt.Errorf("name is required")
	}
	if imageSpec.Spec.SourceUrl == nil || strings.TrimSpace(*imageSpec.Spec.SourceUrl) == "" {
		return "", fmt.Errorf("sourceUrl is required")
	}

	nodeName := ""
	if imageSpec.Metadata.NodeName != nil {
		nodeName = strings.TrimSpace(*imageSpec.Metadata.NodeName)
	}

	apiVersion := strings.TrimSpace(imageSpec.ApiVersion)
	if apiVersion == "" {
		apiVersion = "v1"
	}
	kind := strings.TrimSpace(imageSpec.Kind)
	if kind == "" {
		kind = "Image"
	}

	// 一意なIDを発行
	id, uuidString, err := d.getUniqueImageID()
	if err != nil {
		slog.Error("MakeImageEntryFromSpec()", "err", err)
		return "", err
	}

	sourceURL := strings.TrimSpace(*imageSpec.Spec.SourceUrl)
	spec := imageSpec.Spec
	spec.SourceUrl = &sourceURL

	img := api.Image{
		ApiVersion: apiVersion,
		Kind:       kind,
		Metadata: api.Metadata{
			Name: strings.TrimSpace(imageSpec.Metadata.Name),
			Id:   id,
			Uuid: util.StringPtr(uuidString),
		},
		Spec: spec,
		Status: &api.Status{
			StatusCode:          IMAGE_PENDING,
			Status:              util.StringPtr(ImageStatus[IMAGE_PENDING]),
			CreationTimeStamp:   util.TimePtr(time.Now()),
			LastUpdateTimeStamp: util.TimePtr(time.Now()),
			Message:             util.StringPtr("イメージの作成処理の開始待ち"),
		},
	}
	if nodeName != "" {
		img.Metadata.NodeName = util.StringPtr(nodeName)
	}

	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, img); err != nil {
		slog.Error("MakeImageEntryFromSpec()", "err", err)
		return "", err
	}

	return id, nil
}

// URLのイメージをダウンロードして、それからイメージを作成する。
// nodeName が指定されている場合は、Metadata.nodeName に記録する。
func (d *Database) MakeImageEntryFromURLWithNode(name, url, nodeName string) (string, error) {
	return d.makeImageEntryFromURLWithNodeAndMeta(name, url, nodeName, "v1", "Image")
}

func (d *Database) makeImageEntryFromURLWithNodeAndMeta(name, url, nodeName, apiVersion, kind string) (string, error) {
	slog.Debug("MakeImageEntryFromURL() called", "name", name, "url", url)

	//一意なIDを発行
	id, uuidString, err := d.getUniqueImageID()
	if err != nil {
		slog.Error("MakeImageEntryFromURL()", "err", err)
		return "", err
	}

	//イメージの基本情報を保存
	img := api.Image{
		ApiVersion: apiVersion,
		Kind:       kind,
		Metadata: api.Metadata{
			Name: name,
			Id:   id,
			Uuid: util.StringPtr(uuidString),
		},
		Spec: api.ImageSpec{
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
	if nodeName != "" {
		img.Metadata.NodeName = util.StringPtr(nodeName)
	}
	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, img); err != nil {
		slog.Error("MakeImageEntryFromURL()", "err", err)
		return "", err
	}

	return id, nil
}

// MakeImportedImageEntry は既存のqcow2ファイルを参照する利用可能なイメージを登録する。
func (d *Database) MakeImportedImageEntry(name, nodeName, qcow2Path string) (api.Image, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return api.Image{}, fmt.Errorf("name is required")
	}
	trimmedPath := strings.TrimSpace(qcow2Path)
	if trimmedPath == "" {
		return api.Image{}, fmt.Errorf("qcow2Path is required")
	}

	id, uuidString, err := d.getUniqueImageID()
	if err != nil {
		return api.Image{}, err
	}

	now := time.Now()
	img := api.Image{
		ApiVersion: "v1",
		Kind:       "Image",
		Metadata: api.Metadata{
			Name: trimmedName,
			Id:   id,
			Uuid: util.StringPtr(uuidString),
		},
		Spec: api.ImageSpec{
			Kind:      util.StringPtr("os"),
			Type:      util.StringPtr("qcow2"),
			Qcow2Path: util.StringPtr(trimmedPath),
		},
		Status: &api.Status{
			StatusCode:          IMAGE_PENDING,
			Status:              util.StringPtr(ImageStatus[IMAGE_PENDING]),
			CreationTimeStamp:   util.TimePtr(now),
			LastUpdateTimeStamp: util.TimePtr(now),
			Message:             util.StringPtr("インポート済みイメージの同期準備中"),
		},
	}

	if node := strings.TrimSpace(nodeName); node != "" {
		img.Metadata.NodeName = util.StringPtr(node)
	}

	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, img); err != nil {
		return api.Image{}, err
	}

	return img, nil
}

// ブートボリュームからイメージを作成する
func (d *Database) MakeImageEntryFromRunningVM(serverId, name string) (api.Image, error) {
	slog.Debug("MakeImageEntryFromRunningVM() called", "name", name, "serverId", serverId)

	//一意なIDを発行
	id, uuidString, err := d.getUniqueImageID()
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
	var serverNodeName *string
	if server.Metadata.NodeName != nil {
		node := strings.TrimSpace(*server.Metadata.NodeName)
		if node != "" {
			serverNodeName = util.StringPtr(node)
		}
	}
	if serverNodeName == nil && bootVol.Metadata.NodeName != nil {
		node := strings.TrimSpace(*bootVol.Metadata.NodeName)
		if node != "" {
			slog.Warn("MakeImageEntryFromRunningVM() server metadata nodeName is empty; using boot volume nodeName", "serverId", serverId, "nodeName", node)
			serverNodeName = util.StringPtr(node)
		}
	}
	if serverNodeName == nil {
		err = fmt.Errorf("nodeName is not set for server %s when creating image from running VM", serverId)
		slog.Error("MakeImageEntryFromRunningVM()", "err", err, "serverId", serverId)
		return api.Image{}, err
	}

	// ボリュームがOSボリュームであることを確認
	if *bootVol.Spec.Kind != "os" {
		slog.Error("MakeImageEntryFromRunningVM() volume is not an OS volume", "volumeId", api.VolumeID(*bootVol))
		return api.Image{}, fmt.Errorf("volume with id %v is not an OS volume", api.VolumeID(*bootVol))
	}

	//イメージの基本情報を保存
	labels := map[string]interface{}{
		"source":   "bootVolume",
		"serverId": serverId,
	}
	resolvedOSName, resolvedOSVersion := d.resolveImageOSMetadataFromBootVolume(bootVol, serverNodeName)
	var img api.Image
	if bootVol.Spec.Type != nil && *bootVol.Spec.Type == "qcow2" {
		// イメージのqcow2ボリューム名を設定
		// 実体ディレクトリの作成は、割り当てノードで実行されるコントローラー側で行う。
		imageDir := fmt.Sprintf("/var/lib/marmot/images/%s", id)
		imagePath := fmt.Sprintf("%s/osimage-%s.qcow2", imageDir, id)
		img = api.Image{
			ApiVersion: "v1",
			Kind:       "Image",
			Metadata: api.Metadata{
				Name:     name,
				Labels:   &labels,
				NodeName: serverNodeName,
				Id:       id,
				Uuid:     util.StringPtr(uuidString),
			},
			Spec: api.ImageSpec{
				Kind:          bootVol.Spec.Kind, // ポインタの値を直接使用
				Type:          bootVol.Spec.Type,
				VolumeGroup:   nil,
				LogicalVolume: nil,
				LvPath:        nil,
				Qcow2Path:     util.StringPtr(imagePath),
				OsName:        resolvedOSName,
				OsVersion:     resolvedOSVersion,
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
			ApiVersion: "v1",
			Kind:       "Image",
			Metadata: api.Metadata{
				Name:     name,
				Labels:   &labels,
				NodeName: serverNodeName,
				Id:       id,
				Uuid:     util.StringPtr(uuidString),
			},
			Spec: api.ImageSpec{
				Kind:          bootVol.Spec.Kind, // ポインタの値を直接使用
				Type:          bootVol.Spec.Type,
				VolumeGroup:   bootVol.Spec.VolumeGroup,
				LogicalVolume: util.StringPtr(logicalVolumeName),
				LvPath:        util.StringPtr(logicalVolumePath),
				Qcow2Path:     nil,
				OsName:        resolvedOSName,
				OsVersion:     resolvedOSVersion,
				Size:          bootVol.Spec.Size,
				SourceUrl:     nil,
			},
			Status: &api.Status{
				StatusCode: IMAGE_PENDING,
				Status:     util.StringPtr(ImageStatus[IMAGE_PENDING]),
			},
		}
	} else {
		slog.Error("MakeImageEntryFromRunningVM() unsupported volume type", "volumeId", api.VolumeID(*bootVol), "type", bootVol.Spec.Type)
		return api.Image{}, fmt.Errorf("unsupported volume type for volume with id %v: %v", api.VolumeID(*bootVol), bootVol.Spec.Type)
	}

	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, img); err != nil {
		slog.Error("MakeImageEntryFromRunningVM()", "err", err)
		return api.Image{}, err
	}

	return img, nil
}

func (d *Database) resolveImageOSMetadataFromBootVolume(bootVol *api.Volume, serverNodeName *string) (*string, *string) {
	if bootVol == nil {
		return nil, nil
	}
	variant := ""
	if bootVol.Spec.OsVariant != nil {
		variant = strings.TrimSpace(*bootVol.Spec.OsVariant)
	}
	if variant == "" {
		return nil, nil
	}

	if serverNodeName != nil && strings.TrimSpace(*serverNodeName) != "" {
		sourceImage, err := d.FindImageByNameAndNode(variant, strings.TrimSpace(*serverNodeName))
		if err == nil {
			osName, osVersion := extractImageOSMetadata(sourceImage)
			if osName != nil || osVersion != nil {
				return osName, osVersion
			}
		}
	}

	sourceImage, err := d.FindImageByName(variant)
	if err == nil {
		osName, osVersion := extractImageOSMetadata(sourceImage)
		if osName != nil || osVersion != nil {
			return osName, osVersion
		}
	}

	osName, osVersion := deriveImageOSFromVariant(variant)
	if osName == "" && osVersion == "" {
		return nil, nil
	}

	namePtr, versionPtr := (*string)(nil), (*string)(nil)
	if osName != "" {
		namePtr = util.StringPtr(osName)
	}
	if osVersion != "" {
		versionPtr = util.StringPtr(osVersion)
	}
	return namePtr, versionPtr
}

func extractImageOSMetadata(image api.Image) (*string, *string) {
	var osName, osVersion *string
	if image.Spec.OsName != nil {
		name := strings.TrimSpace(*image.Spec.OsName)
		if name != "" {
			osName = util.StringPtr(name)
		}
	}
	if image.Spec.OsVersion != nil {
		version := strings.TrimSpace(*image.Spec.OsVersion)
		if version != "" {
			osVersion = util.StringPtr(version)
		}
	}
	return osName, osVersion
}

func deriveImageOSFromVariant(osVariant string) (string, string) {
	v := strings.ToLower(strings.TrimSpace(osVariant))
	switch {
	case strings.HasPrefix(v, "ubuntu22.04"):
		return "ubuntu", "22.04"
	case strings.HasPrefix(v, "ubuntu24.04"):
		return "ubuntu", "24.04"
	case strings.HasPrefix(v, "alpine3.23"):
		return "alpine", "3.23"
	default:
		return "", ""
	}
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
	normalizeImageMetadataID(&img, id)

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
		normalizeImageMetadataID(&img, imageIDFromKey(string(kv.Key)))
		images = append(images, img)
	}

	return images, nil
}

func imageIDFromKey(key string) string {
	prefix := ImagePrefix + "/"
	id := strings.TrimPrefix(strings.TrimSpace(key), prefix)
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[:i]
	}
	return strings.TrimSpace(id)
}

func normalizeImageMetadataID(img *api.Image, fallbackID string) {
	if img == nil {
		return
	}
	if strings.TrimSpace(img.ApiVersion) == "" {
		img.ApiVersion = "v1"
	}
	if strings.TrimSpace(img.Kind) == "" {
		img.Kind = "Image"
	}
	fallbackID = strings.TrimSpace(fallbackID)
	if fallbackID == "" {
		return
	}
	if strings.TrimSpace(img.Metadata.Id) == "" {
		img.Metadata.Id = fallbackID
	}
}

// IDで特定してイメージを削除する
func (d *Database) DeleteImage(id string) error {
	slog.Debug("DeleteImages() called", "id", id)
	key := ImagePrefix + "/" + id
	return d.DeleteJSON(key)
}

// ステータスの変更
func (d *Database) UpdateImageStatus(id string, status int) error {
	return d.UpdateImageStatusMessage(id, status, "")
}

// ステータスとメッセージの変更
func (d *Database) UpdateImageStatusMessage(id string, status int, message string) error {
	for {
		err := d.updateImageStatusMessage(id, status, message)
		if err == ErrUpdateConflict {
			slog.Warn("UpdateImageStatusMessage() retrying due to update conflict", "imageId", id)
			continue
		}
		if err != nil {
			slog.Error("UpdateImageStatusMessage() failed", "err", err, "imageId", id)
			return err
		}
		return nil
	}
}

func (d *Database) updateImageStatusMessage(id string, status int, message string) error {
	lockKey := "/lock/image/" + id
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var rec api.Image
	key := ImagePrefix + "/" + id
	resp, err := d.GetJSON(key, &rec)
	if err != nil {
		return err
	}

	if rec.Status == nil {
		rec.Status = &api.Status{}
	}
	rec.Status.StatusCode = status
	rec.Status.Status = util.StringPtr(ImageStatus[status])
	rec.Status.Message = nil
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		rec.Status.Message = util.StringPtr(trimmed)
	}
	rec.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())

	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, rec)
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

	debugPrintln("=== 書き込みデータの情報確認 ===", "image Id", id)
	data3, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		slog.Error("json.MarshalIndent()", "err", err)
	} else {
		debugPrintln("イメージ情報(image): ", string(data3))
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

	rec.Metadata.Id = id
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
	key := ImagePrefix + "/" + id
	var image api.Image
	resp, err := d.GetJSON(key, &image)
	if err != nil {
		slog.Error("SetDeleteTimestamp() GetImage() failed", "err", err, "imageId", id)
		return err
	}
	image.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	image.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	if err := d.PutJSONCAS(key, resp.Kvs[0].ModRevision, image); err != nil {
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
		if img.Metadata.Name == name {
			return img, nil
		}
	}

	return api.Image{}, fmt.Errorf("image not found with name: %v", name)
}

// イメージ名とノード名でイメージを検索する。
// NodeName が一致するエントリーのみ返す。
func (d *Database) FindImageByNameAndNode(name, nodeName string) (api.Image, error) {
	slog.Debug("FindImageByNameAndNode() called", "name", name, "nodeName", nodeName)

	targetNode := strings.TrimSpace(nodeName)
	if targetNode == "" {
		return api.Image{}, fmt.Errorf("nodeName is required")
	}

	resp, err := d.GetByPrefix(ImagePrefix)
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
			slog.Error("FindImageByNameAndNode() failed to unmarshal image", "err", err)
			continue
		}

		if strings.TrimSpace(img.Metadata.Name) == "" {
			continue
		}
		if strings.TrimSpace(img.Metadata.Name) != strings.TrimSpace(name) {
			continue
		}
		if img.Metadata.NodeName == nil || strings.TrimSpace(*img.Metadata.NodeName) == "" {
			continue
		}

		if strings.TrimSpace(*img.Metadata.NodeName) != targetNode {
			continue
		}

		return img, nil
	}

	return api.Image{}, fmt.Errorf("image not found with name: %v and nodeName: %v", name, targetNode)
}

// MakeFollowerImageEntry creates a follower-side image object that waits for the head image.
// The follower image keeps headImageId in labels and starts from WAITING status.
func (d *Database) MakeFollowerImageEntry(headImage api.Image, followerNodeName string, headImageId string) (string, error) {
	followerNodeName = strings.TrimSpace(followerNodeName)
	headImageId = strings.TrimSpace(headImageId)
	if followerNodeName == "" {
		return "", fmt.Errorf("follower nodeName is required")
	}
	if headImageId == "" {
		return "", fmt.Errorf("head image id is required")
	}
	if strings.TrimSpace(headImage.Metadata.Name) == "" {
		return "", fmt.Errorf("head image metadata.name is required")
	}

	id, uuidString, err := d.getUniqueImageID()
	if err != nil {
		slog.Error("MakeFollowerImageEntry() getUniqueImageID failed", "err", err)
		return "", err
	}

	labels := make(map[string]interface{})

	// Set sync-related labels using helpers
	headNodeName := ""
	if headImage.Metadata.NodeName != nil {
		headNodeName = *headImage.Metadata.NodeName
	}
	SetFollowerSyncLabels(labels, "follower", headImageId, headNodeName)

	// Set source-related labels from head image if present
	if headImage.Metadata.Labels != nil {
		source := GetImageSource(*headImage.Metadata.Labels)
		serverID := GetImageServerID(*headImage.Metadata.Labels)
		SetImageSourceLabels(labels, source, serverID)
	}

	imageDir := fmt.Sprintf("/var/lib/marmot/images/%s", id)
	imagePath := fmt.Sprintf("%s/osimage-%s.qcow2", imageDir, id)

	followerSpec := api.ImageSpec{
		Kind:      util.StringPtr("os"),
		Type:      util.StringPtr("qcow2"),
		Qcow2Path: util.StringPtr(imagePath),
		SourceUrl: nil,
	}
	if headImage.Spec.Kind != nil {
		followerSpec.Kind = util.StringPtr(*headImage.Spec.Kind)
	}
	if headImage.Spec.Type != nil {
		followerSpec.Type = util.StringPtr(*headImage.Spec.Type)
	}
	if headImage.Spec.Size != nil {
		followerSpec.Size = util.IntPtrInt(*headImage.Spec.Size)
	}
	if headImage.Spec.SourceUrl != nil {
		sourceURL := strings.TrimSpace(*headImage.Spec.SourceUrl)
		if sourceURL != "" {
			followerSpec.SourceUrl = util.StringPtr(sourceURL)
		}
	}
	if headImage.Spec.OsName != nil {
		osName := strings.TrimSpace(*headImage.Spec.OsName)
		if osName != "" {
			followerSpec.OsName = util.StringPtr(osName)
		}
	}
	if headImage.Spec.OsVersion != nil {
		osVersion := strings.TrimSpace(*headImage.Spec.OsVersion)
		if osVersion != "" {
			followerSpec.OsVersion = util.StringPtr(osVersion)
		}
	}

	follower := api.Image{
		ApiVersion: "v1",
		Kind:       "Image",
		Metadata: api.Metadata{
			Name:     headImage.Metadata.Name,
			NodeName: util.StringPtr(followerNodeName),
			Labels:   &labels,
			Id:       id,
			Uuid:     util.StringPtr(uuidString),
		},
		Spec: followerSpec,
		Status: &api.Status{
			StatusCode:          IMAGE_WAITING,
			Status:              util.StringPtr(ImageStatus[IMAGE_WAITING]),
			CreationTimeStamp:   util.TimePtr(time.Now()),
			LastUpdateTimeStamp: util.TimePtr(time.Now()),
			Message:             util.StringPtr("ヘッドノードのイメージ作成完了を待機中"),
		},
	}

	key := ImagePrefix + "/" + id
	if err := d.PutJSON(key, follower); err != nil {
		slog.Error("MakeFollowerImageEntry() PutJSON failed", "err", err)
		return "", err
	}

	return id, nil
}
