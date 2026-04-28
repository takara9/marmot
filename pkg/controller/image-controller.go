package controller

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
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
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	// 定期実行の開始
	ticker := time.NewTicker(IMAGE_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.imageControllerLoop()
			case <-c.stopChan:
				slog.Info("イメージコントローラー停止")
				return
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
		if ok, assignedNode, reason := evaluateNodeAssignment(image.Metadata, c.marmot.NodeName); !ok {
			objectName := ""
			if image.Metadata != nil && image.Metadata.Name != nil {
				objectName = *image.Metadata.Name
			}
			slog.Debug("別ノード割当のイメージをスキップ", "imageId", image.Id, "imageName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

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
			if err := c.ensureFollowerImagesWaiting(image); err != nil {
				slog.Error("フォロワー用イメージエントリーの作成に失敗", "headImageId", image.Id, "err", err)
			}
			c.marmot.Db.UpdateImageStatus(image.Id, db.IMAGE_CREATING)
			// ラベルの存在をチェック
			if image.Metadata.Labels != nil {
				source := db.GetImageSource(*image.Metadata.Labels)
				if source == "bootVolume" {
					slog.Debug("実行中VMからイメージの作成", "image", *image.Metadata.Name, "source", "bootVolume")
					serverId := db.GetImageServerID(*image.Metadata.Labels)
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
		case db.IMAGE_WAITING:
			slog.Debug("イメージはヘッドノード完了待ち", "image", *image.Metadata.Name)
			if err := c.startFollowerSync(image); err != nil {
				slog.Error("フォロワーイメージの同期開始に失敗", "imageId", image.Id, "err", err)
			}
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

func (c *controller) ensureFollowerImagesWaiting(headImage api.Image) error {
	if headImage.Metadata == nil || headImage.Metadata.Name == nil {
		return nil
	}

	headNode := ""
	if headImage.Metadata.NodeName != nil {
		headNode = strings.TrimSpace(*headImage.Metadata.NodeName)
	}
	if headNode == "" {
		return fmt.Errorf("head image nodeName is empty: imageId=%s", headImage.Id)
	}

	nodeStatuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		return err
	}

	nodes := make(map[string]struct{}, len(nodeStatuses))
	for _, status := range nodeStatuses {
		if status.NodeName == nil {
			continue
		}
		node := strings.TrimSpace(*status.NodeName)
		if node == "" || node == headNode {
			continue
		}
		nodes[node] = struct{}{}
	}

	for followerNode := range nodes {
		newFollowerId, createErr := c.marmot.Db.MakeFollowerImageEntry(headImage, followerNode, headImage.Id)
		if createErr != nil {
			slog.Error("フォロワーイメージエントリー作成失敗", "headImageId", headImage.Id, "followerNode", followerNode, "err", createErr)
			continue
		}
		slog.Debug("フォロワーイメージをWAITINGで登録", "headImageId", headImage.Id, "followerImageId", newFollowerId, "followerNode", followerNode)
	}

	return nil
}

func (c *controller) startFollowerSync(waitingImage api.Image) error {
	if waitingImage.Metadata == nil || waitingImage.Metadata.Labels == nil {
		return fmt.Errorf("labels are required for waiting image: imageId=%s", waitingImage.Id)
	}
	labels := *waitingImage.Metadata.Labels
	headImageID := db.GetHeadImageID(labels)
	if headImageID == "" {
		return fmt.Errorf("headImageId label is missing: imageId=%s", waitingImage.Id)
	}

	headImage, err := c.marmot.Db.GetImage(headImageID)
	if err != nil {
		return err
	}
	if headImage.Status == nil {
		return fmt.Errorf("head image status is nil: headImageId=%s", headImageID)
	}

	switch headImage.Status.StatusCode {
	case db.IMAGE_AVAILABLE:
		c.marmot.Db.UpdateImageStatusMessage(waitingImage.Id, db.IMAGE_CREATING, "ヘッドノードからQCOW2イメージを取得中")
		go func(image api.Image, head api.Image) {
			if err := c.syncFollowerImageFromHead(image, head); err != nil {
				slog.Error("フォロワーイメージ同期に失敗", "imageId", image.Id, "headImageId", head.Id, "err", err)
				c.marmot.Db.UpdateImageStatusMessage(image.Id, db.IMAGE_CREATION_FAILED, err.Error())
			}
		}(waitingImage, headImage)
	case db.IMAGE_CREATION_FAILED, db.IMAGE_DELETED:
		msg := fmt.Sprintf("head image is not available: headImageId=%s status=%s", headImage.Id, util.OrDefault(headImage.Status.Status, ""))
		c.marmot.Db.UpdateImageStatusMessage(waitingImage.Id, db.IMAGE_CREATION_FAILED, msg)
	default:
		// WAITING を維持する。
	}

	return nil
}

func (c *controller) syncFollowerImageFromHead(followerImage api.Image, headImage api.Image) error {
	if headImage.Metadata == nil || headImage.Metadata.NodeName == nil {
		return fmt.Errorf("head image nodeName is required: headImageId=%s", headImage.Id)
	}
	headNode := strings.TrimSpace(*headImage.Metadata.NodeName)
	if headNode == "" {
		return fmt.Errorf("head image nodeName is empty: headImageId=%s", headImage.Id)
	}

	headHostStatus, err := c.marmot.Db.GetHostStatus(headNode)
	if err != nil {
		return fmt.Errorf("failed to get head node host status: node=%s err=%w", headNode, err)
	}
	if headHostStatus.IpAddress == nil || strings.TrimSpace(*headHostStatus.IpAddress) == "" {
		return fmt.Errorf("head node ipAddress is empty: node=%s", headNode)
	}
	headIP := strings.TrimSpace(*headHostStatus.IpAddress)
	headURL := buildHeadImageDownloadURL(headIP, headImage.Id)

	followerLatest, err := c.marmot.Db.GetImage(followerImage.Id)
	if err != nil {
		return err
	}
	if followerLatest.Spec == nil {
		followerLatest.Spec = &api.ImageSpec{}
	}

	destinationPath := ""
	if followerLatest.Spec.Qcow2Path != nil {
		destinationPath = strings.TrimSpace(*followerLatest.Spec.Qcow2Path)
	}
	if destinationPath == "" {
		imageDir := filepath.Join("/var/lib/marmot/images", followerLatest.Id)
		destinationPath = filepath.Join(imageDir, fmt.Sprintf("osimage-%s.qcow2", followerLatest.Id))
		followerLatest.Spec.Qcow2Path = util.StringPtr(destinationPath)
	}

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return fmt.Errorf("failed to create follower image directory: %w", err)
	}

	timeout := marmotd.CurrentConfig().ImageDownloadTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := downloadImageFromHeadWithContext(ctx, headURL, destinationPath); err != nil {
		return err
	}

	followerLatest.Spec.Kind = util.StringPtr("os")
	followerLatest.Spec.Type = util.StringPtr("qcow2")
	followerLatest.Spec.SourceUrl = nil
	if headImage.Spec != nil && headImage.Spec.Size != nil {
		followerLatest.Spec.Size = util.IntPtrInt(*headImage.Spec.Size)
	}
	if followerLatest.Metadata != nil && followerLatest.Metadata.Labels != nil {
		labels := *followerLatest.Metadata.Labels
		db.SetFollowerSyncLabels(labels, "follower", headImage.Id, headNode)
		followerLatest.Metadata.Labels = &labels
	}

	followerLatest.Status.StatusCode = db.IMAGE_AVAILABLE
	followerLatest.Status.Status = util.StringPtr(db.ImageStatus[db.IMAGE_AVAILABLE])
	followerLatest.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
	followerLatest.Status.Message = nil

	if err := c.marmot.Db.UpdateImage(followerLatest.Id, followerLatest); err != nil {
		return err
	}

	return nil
}

func buildHeadImageDownloadURL(headIP, imageID string) string {
	port := "8750"
	apiListenAddr := strings.TrimSpace(marmotd.CurrentConfig().APIListenAddr)
	if host, p, err := net.SplitHostPort(apiListenAddr); err == nil {
		if strings.TrimSpace(p) != "" {
			port = p
		}
		_ = host
	} else if strings.HasPrefix(apiListenAddr, ":") {
		if p := strings.TrimPrefix(apiListenAddr, ":"); strings.TrimSpace(p) != "" {
			port = p
		}
	}

	return fmt.Sprintf("http://%s:%s/api/v1/image/%s/qcow2", headIP, port, imageID)
}

func downloadImageFromHeadWithContext(ctx context.Context, sourceURL, destPath string) error {
	client := &http.Client{Timeout: marmotd.CurrentConfig().ImageDownloadTimeout()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch head image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("head image fetch failed: status=%s", resp.Status)
	}

	tmpPath := destPath + ".part"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to copy response body: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to move temp file: %w", err)
	}

	return nil
}
