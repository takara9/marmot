package controller

import (
	"context"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	IMAGE_CONTROLLER_INTERVAL = 5 * time.Second
)

// イメージコントローラーの開始
// deletionDelaySeconds に 0 を渡した場合はデフォルト値 (10秒) が使用されます。
func StartImageController(node string, etcdUrl string, deletionDelaySeconds int) (*controller, error) {
	var c controller
	var err error

	if deletionDelaySeconds <= 0 {
		deletionDelaySeconds = 10
	}
	c.deletionDelay = time.Duration(deletionDelaySeconds) * time.Second

	// 初期化
	// marmotd との接続設定
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db // 正しくないけど

	// 定期実行の開始
	ticker := time.NewTicker(IMAGE_CONTROLLER_INTERVAL)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.imageControllerLoop()
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) imageControllerLoop() {
	slog.Debug("イメージコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format(time.DateTime))

	imgaes, err := c.marmot.GetImagesManage()
	if err != nil {
		slog.Error("failed to get images", "err", err)
		return
	}

	for _, image := range imgaes {
		// 削除タイムスタンプの処理
		// 削除のタイムスタンプが一定時間以上経過しているかをチェックして、削除処理を実行する
		if image.Status != nil && image.Status.DeletionTimeStamp != nil {
			deletionTime := *image.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているイメージ検出", "IMAGE", *image.Metadata.Name)
				c.marmot.Db.UpdateImageStatus(image.Id, db.IMAGE_DELETING)
			}
		}

		//jsonBytes, err := json.MarshalIndent(image, "", "    ")
		//if err != nil {
		//	slog.Error("failed to marshal image", "err", err)
		//	continue
		//}
		//fmt.Println("details", string(jsonBytes))
		slog.Debug("イメージの状態を確認", "image", *image.Metadata.Name, "state", db.ImageStatus[image.Status.StatusCode])

		// イメージの状態に応じた処理
		switch image.Status.StatusCode {
		case db.IMAGE_PENDING:
			slog.Debug("イメージの作成処理を実行", "image", *image.Metadata.Name)
			c.marmot.Db.UpdateImageStatus(image.Id, db.IMAGE_CREATING)
			// ラベルの存在をチェック,
			if image.Metadata.Labels != nil && (*image.Metadata.Labels)["source"] == "bootVolume" {
				slog.Debug("実行中VMからイメージの作成", "image", *image.Metadata.Name, "source", "bootVolume")
				serverId := (*image.Metadata.Labels)["serverId"].(string)
				go func(image api.Image, serverId string) {
					timeout := marmotd.CurrentConfig().ImageCreateFromVMTimeout()
					ctx, cancel := context.WithTimeout(context.Background(), timeout)
					defer cancel()
					if _, err := c.marmot.MakeImageEntryFromRunningVMWithContext(ctx, serverId, *image.Metadata.Name, image); err != nil {
						slog.Error("実行中VMからのイメージ作成に失敗", "imageId", image.Id, "serverId", serverId, "timeout", timeout, "err", err)
					}
				}(image, serverId)
			} else {
				slog.Debug("ダウンロードしてイメージの作成", "image", *image.Metadata.Name, "source", "url")
				go func(image api.Image) {
					timeout := marmotd.CurrentConfig().ImageCreateFromURLTimeout()
					ctx, cancel := context.WithTimeout(context.Background(), timeout)
					defer cancel()
					if _, err := c.marmot.CreateNewImageManageWithContext(ctx, image.Id); err != nil {
						slog.Error("URLからのイメージ作成に失敗", "imageId", image.Id, "timeout", timeout, "err", err)
					}
				}(image)
			}
		case db.IMAGE_CREATING:
			slog.Debug("イメージの作成処理を継続", "image", *image.Metadata.Name)
			// ここにイメージの作成処理の継続を実装
		case db.IMAGE_CREATION_FAILED:
			slog.Debug("イメージの作成に失敗", "image", *image.Metadata.Name)
			// ここにイメージの作成失敗時の処理を実装
		case db.IMAGE_AVAILABLE:
			slog.Debug("イメージは利用可能", "image", *image.Metadata.Name)
			if err := marmotd.CheckImageBackingStore(image); err != nil {
				slog.Warn("AVAILABLE イメージの実体が見つからないため DELETED に更新", "imageId", image.Id, "err", err)
				c.marmot.Db.UpdateImageStatusMessage(image.Id, db.IMAGE_DELETED, err.Error())
			}
		case db.IMAGE_DELETING:
			slog.Debug("イメージの削除処理を実行", "image", *image.Metadata.Name)
			err := c.marmot.DeleteImageManage(image.Id)
			if err != nil {
				slog.Error("DeleteImageById()", "err", err)
			}
		default:
			slog.Debug("イメージは安定状態", "image", *image.Metadata.Name, "state", *image.Status.Status)
		}
	}
}
