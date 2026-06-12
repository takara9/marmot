package controller

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	VOLUME_CONTROLLER_INTERVAL = 5 * time.Second
	VOLUME_STALE_TIMEOUT       = 10 * time.Minute
)

/*
var controllerCounter uint64 = 0

type controller struct {
	db     *db.Database
	Lock   sync.Mutex
	marmot *marmotd.Marmot
}
*/

// ボリュームコントローラーの開始
// deletionDelaySeconds に 0 を渡した場合はデフォルト値 (10秒) が使用されます。
func StartVolController(node string, etcdUrl string, deletionDelaySeconds int) (*controller, error) {
	var c controller
	var err error

	if deletionDelaySeconds <= 0 {
		deletionDelaySeconds = 10
	}
	c.deletionDelay = time.Duration(deletionDelaySeconds) * time.Second

	// marmotd との接続設定
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	// 定期実行の開始
	ticker := time.NewTicker(VOLUME_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.volumeControllerLoop()
			case <-c.stopChan:
				slog.Info("ボリュームコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) volumeControllerLoop() {
	slog.Debug("ボリュームコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	statuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		slog.Warn("GetAllHostStatus() failed; ボリュームの nodeName 存在確認をスキップ", "err", err)
		statuses = nil
	}

	vols, err := c.marmot.GetVolumes()
	if err != nil {
		slog.Error("failed to get volumes", "err", err)
		return
	}
	slog.Debug("取得したボリュームの数", "numVolumes", len(vols))
	for _, vol := range vols {
		volID := api.VolumeID(vol)
		if shouldFail, message := shouldFailVolumeForMissingAssignedNode(vol, statuses); shouldFail {
			assignedNode := ""
			if vol.Metadata.NodeName != nil {
				assignedNode = strings.TrimSpace(*vol.Metadata.NodeName)
			}
			slog.Warn("存在しない nodeName 指定のためボリュームを ERROR に更新", "volId", volID, "assignedNode", assignedNode, "message", message)
			c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, message)
			continue
		}

		if shouldDelete, reason := shouldDeleteVolumeForMissingAssignedNode(vol, statuses); shouldDelete {
			assignedNode := ""
			if vol.Metadata.NodeName != nil {
				assignedNode = strings.TrimSpace(*vol.Metadata.NodeName)
			}
			slog.Warn("存在しない nodeName 指定のためボリューム定義を削除", "volId", volID, "assignedNode", assignedNode, "reason", reason)
			if err := c.db.DeleteVolume(volID); err != nil {
				slog.Error("DeleteVolume()", "err", err, "volId", volID)
			}
			continue
		}

		if ok, assignedNode, reason := evaluateNodeAssignment(&vol.Metadata, c.marmot.NodeName); !ok {
			objectName := vol.Metadata.Name
			slog.Debug("別ノード割当のボリュームをスキップ", "volumeId", volID, "volumeName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			slog.Debug("ボリュームの詳細情報", "volumeId", volID, "volumeName", objectName, "metadata", vol.Metadata, "status", vol.Status)
			continue
		}

		// デバッグ
		//byte, err := json.MarshalIndent(vol, "", "  ")
		//if err != nil {
		//	slog.Error("failed to marshal volume", "err", err)
		//} else {
		//	fmt.Println("ボリュームのJSON情報", "json", string(byte))
		//}

		// 削除タイムスタンプが設定されて一定時間経過したボリュームのステータスをDELETINGに更新する
		if vol.Status != nil && vol.Status.DeletionTimeStamp != nil {
			deletionTime := *vol.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているボリューム検出", "volId", volID)
				c.marmot.Db.UpdateVolumeStatus(volID, db.VOLUME_DELETING)
			}
		}

		// 最終更新から10分以上 ERROR / PROVISIONING が継続している場合は実体ごと自動削除
		if vol.Status != nil && vol.Status.LastUpdateTimeStamp != nil {
			isStale := time.Since(*vol.Status.LastUpdateTimeStamp) > VOLUME_STALE_TIMEOUT
			isTargetState := vol.Status.StatusCode == db.VOLUME_ERROR || vol.Status.StatusCode == db.VOLUME_PROVISIONING
			if isStale && isTargetState {
				slog.Warn("最終更新から10分以上放置されたボリュームを削除キューへ登録", "volId", volID, "status", vol.Status.StatusCode, "lastUpdate", vol.Status.LastUpdateTimeStamp)
				if vol.Status.DeletionTimeStamp == nil {
					c.db.SetVolumeDeletionTimestamp(volID)
					slog.Info("放置ボリュームにDeletionTimeStampを設定", "volId", volID)
				}
				continue
			}
		}

		slog.Debug("ボリュームの情報", "volId", volID, "volName", vol.Metadata.Name, "volStatus", *vol.Status.Status)
		switch vol.Status.StatusCode {
		case db.VOLUME_PENDING:
			slog.Debug("待ち状態のボリュームを処理", "volId", volID)
			// 待ち状態のボリュームを処理するコードをここに追加
			c.db.UpdateVolumeStatus(volID, db.VOLUME_PROVISIONING)
			if _, err := c.marmot.CreateNewVolume(volID); err != nil {
				slog.Error("CreateNewVolume()", "err", err)
				c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, err.Error())
				continue
			}

			isISCSIVolume := vol.Spec.Type != nil && *vol.Spec.Type == "lvm" &&
				vol.Spec.Kind != nil && *vol.Spec.Kind == "data" &&
				vol.Spec.Iscsi != nil && *vol.Spec.Iscsi
			if isISCSIVolume {
				if err := c.marmot.ConfigureISCSIForVolumeByID(volID); err != nil {
					slog.Error("ConfigureISCSIForVolumeByID()", "err", err, "volId", volID)
					c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, err.Error())
					continue
				}
			}

			c.db.UpdateVolumeStatus(volID, db.VOLUME_AVAILABLE)
		case db.VOLUME_PROVISIONING:
			slog.Debug("プロビジョニング中のボリュームを処理", "volId", volID)

		case db.VOLUME_DELETING:
			slog.Debug("削除中のボリュームを処理", "volId", volID)
			shouldCleanupISCSI := vol.Spec.Type != nil && *vol.Spec.Type == "lvm" &&
				vol.Spec.Kind != nil && *vol.Spec.Kind == "data" &&
				((vol.Spec.Iscsi != nil && *vol.Spec.Iscsi) ||
					(vol.Spec.IscsiTargetIqn != nil && strings.TrimSpace(*vol.Spec.IscsiTargetIqn) != ""))
			if shouldCleanupISCSI {
				if err := c.marmot.CleanupISCSIForVolumeByID(volID); err != nil {
					errMsg := strings.ToLower(err.Error())
					if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "no such") || strings.Contains(errMsg, "does not exist") {
						slog.Warn("iSCSI公開解除対象が見つからないため処理を継続", "volId", volID, "err", err)
					} else {
						slog.Error("CleanupISCSIForVolumeByID()", "err", err, "volId", volID)
						c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, err.Error())
						continue
					}
				}
			}

			if err := c.marmot.RemoveVolume(volID); err != nil {
				errMsg := strings.ToLower(err.Error())
				if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "no such") || strings.Contains(errMsg, "does not exist") {
					slog.Warn("削除対象の実体が見つからないためオブジェクト削除を継続", "volId", volID, "err", err)
				} else {
					slog.Error("RemoveVolume()", "err", err)
					c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, err.Error())
					continue
				}
			}
			c.db.DeleteVolume(volID)
			slog.Debug("ボリュームの削除成功", "volId", volID)

		case db.VOLUME_ERROR:
			slog.Debug("エラー状態のボリュームを処理", "volId", volID)
			// エラー状態のボリュームを処理するコードをここに追加
		case db.VOLUME_UNAVAILABLE:
			slog.Debug("実体欠損状態のボリュームを処理", "volId", volID)
		case db.VOLUME_AVAILABLE:
			slog.Debug("利用可能なボリュームを処理", "volId", volID)
			if err := marmotd.CheckVolumeBackingStore(vol); err != nil {
				slog.Warn("AVAILABLE ボリュームの実体が見つからないため UNAVAILABLE に更新", "volId", volID, "err", err)
				c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_UNAVAILABLE, err.Error())
			}
		default:
			slog.Warn("不明なステータスのボリュームをスキップ", "volId", volID, "status", *vol.Status.Status)
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
}

func shouldDeleteVolumeForMissingAssignedNode(vol api.Volume, statuses []api.HostStatus) (bool, string) {
	if vol.Status == nil || vol.Status.StatusCode != db.VOLUME_DELETING {
		return false, ""
	}

	if vol.Metadata.NodeName == nil {
		return false, ""
	}

	assignedNode := strings.TrimSpace(*vol.Metadata.NodeName)
	if assignedNode == "" {
		return false, ""
	}

	if !clusterHasAnyNode(statuses) {
		return false, ""
	}

	if clusterHasNode(statuses, assignedNode) {
		return false, ""
	}

	return true, "assigned_node_not_found"
}

func shouldFailVolumeForMissingAssignedNode(vol api.Volume, statuses []api.HostStatus) (bool, string) {
	if vol.Status == nil {
		return false, ""
	}

	if vol.Status.StatusCode != db.VOLUME_PENDING && vol.Status.StatusCode != db.VOLUME_PROVISIONING {
		return false, ""
	}

	if vol.Metadata.NodeName == nil {
		return false, ""
	}

	assignedNode := strings.TrimSpace(*vol.Metadata.NodeName)
	if assignedNode == "" {
		return false, ""
	}

	if !clusterHasAnyNode(statuses) {
		return false, ""
	}

	if clusterHasNode(statuses, assignedNode) {
		return false, ""
	}

	return true, fmt.Sprintf("metadata.nodeName %q is not found in cluster", assignedNode)
}
