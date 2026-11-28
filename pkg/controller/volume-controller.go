package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	VOLUME_CONTROLLER_INTERVAL = 5 * time.Second
	VOLUME_STALE_TIMEOUT       = 10 * time.Minute

	volumeWatchMinInterval   = time.Second
	volumeWatchRetryInterval = time.Second

	// 作成済みの実体の状態(AVAILABLE)を記録するための即時の再試行回数と間隔
	volumeStatusWriteAttempts      = 3
	volumeStatusWriteRetryInterval = 200 * time.Millisecond
)

// volumeController はボリュームの状態を管理する専用コントローラー。
type volumeController struct {
	db            *db.Database
	marmot        *marmotd.Marmot
	deletionDelay time.Duration // DeletionTimestamp 検知から削除実行までの待機時間
	stopChan      chan struct{}
	doneChan      chan struct{}
	stopOnce      sync.Once
	// 実体は作成済みだが AVAILABLE を記録できていないボリューム。制御ループからのみ参照する。
	// 再試行で記録できるまで、古いボリュームの自動削除から除外する。
	unrecordedVolumes map[string]struct{}
}

// volumeProvisionOps は PENDING ボリュームの作成で使う操作。単体テストで差し替えられるようにする。
type volumeProvisionOps struct {
	setStatus        func(id string, status int) error
	setStatusMessage func(id string, status int, message string) error
	create           func(id string) error
	configureISCSI   func(id string) error
}

// Stop はコントローラーの処理を停止し、終了を待機する。
func (c *volumeController) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		if c.stopChan != nil {
			close(c.stopChan)
		}
	})
	if c.doneChan != nil {
		<-c.doneChan
	}
}

// ボリュームコントローラーの開始
// deletionDelaySeconds に 0 を渡した場合はデフォルト値 (10秒) が使用されます。
func StartVolController(node string, etcdUrl string, deletionDelaySeconds int) (*volumeController, error) {
	var c volumeController
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
	c.unrecordedVolumes = make(map[string]struct{})
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	go func() {
		defer close(c.doneChan)
		ctx, cancel := context.WithCancel(context.Background())
		notifications := make(chan struct{}, 1)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			watchResourceChanges(ctx, c.db.Cli.Watch, db.VolumePrefix, "Volume", notifications, volumeWatchRetryInterval)
		}()
		runServerController(c.stopChan, notifications, VOLUME_CONTROLLER_INTERVAL, volumeWatchMinInterval, c.volumeControllerLoop)
		cancel()
		<-watchDone
		slog.Debug("ボリュームコントローラー停止")
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *volumeController) volumeControllerLoop() {
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
	c.forgetMissingUnrecordedVolumes(vols)
	for _, vol := range vols {
		volID := api.VolumeID(vol)
		if shouldFail, message := shouldFailVolumeForMissingAssignedNode(vol, statuses); shouldFail {
			assignedNode := ""
			if vol.Metadata.NodeName != nil {
				assignedNode = strings.TrimSpace(*vol.Metadata.NodeName)
			}
			slog.Warn("存在しない nodeName 指定のためボリュームを ERROR に更新", "volId", volID, "assignedNode", assignedNode, "message", message)
			if err := c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, message); err != nil {
				slog.Error("UpdateVolumeStatusMessage() failed; 次の周期で再試行します", "volId", volID, "status", db.VOLUME_ERROR, "err", err)
			}
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

		if vol.Status != nil && vol.Status.StatusCode == db.VOLUME_DELETING {
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
						if dbErr := c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, err.Error()); dbErr != nil {
							slog.Error("UpdateVolumeStatusMessage() failed; DELETING のまま次の周期で再試行します", "volId", volID, "status", db.VOLUME_ERROR, "err", dbErr)
						}
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
					if dbErr := c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_ERROR, err.Error()); dbErr != nil {
						slog.Error("UpdateVolumeStatusMessage() failed; DELETING のまま次の周期で再試行します", "volId", volID, "status", db.VOLUME_ERROR, "err", dbErr)
					}
					continue
				}
			}
			if err := c.db.DeleteVolume(volID); err != nil {
				slog.Error("DeleteVolume() failed; 次の周期で再試行します", "err", err, "volId", volID)
				continue
			}
			slog.Debug("ボリュームの削除成功", "volId", volID)
			continue
		}
		if vol.Status != nil && vol.Status.DeletionTimeStamp != nil {
			deletionTime := *vol.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているボリューム検出", "volId", volID)
				if err := c.marmot.Db.UpdateVolumeStatus(volID, db.VOLUME_DELETING); err != nil {
					// 削除予定のボリュームを、古い状態のまま作成・利用する処理へ進めない
					slog.Error("UpdateVolumeStatus() failed; 次の周期で再試行します", "volId", volID, "status", db.VOLUME_DELETING, "err", err)
					continue
				}
			}
		}

		// 実体は作成済みだが AVAILABLE を記録できていないボリュームは、記録を再試行し、自動削除しない
		if c.retryUnrecordedVolume(vol, c.db.UpdateVolumeStatus) {
			continue
		}

		// 最終更新から10分以上 ERROR / PROVISIONING が継続している場合は実体ごと自動削除
		if vol.Status != nil && vol.Status.LastUpdateTimeStamp != nil {
			isStale := time.Since(*vol.Status.LastUpdateTimeStamp) > VOLUME_STALE_TIMEOUT
			isTargetState := vol.Status.StatusCode == db.VOLUME_ERROR || vol.Status.StatusCode == db.VOLUME_PROVISIONING
			if isStale && isTargetState {
				slog.Warn("最終更新から10分以上放置されたボリュームを削除キューへ登録", "volId", volID, "status", vol.Status.StatusCode, "lastUpdate", vol.Status.LastUpdateTimeStamp)
				if vol.Status.DeletionTimeStamp == nil {
					if err := c.db.SetVolumeDeletionTimestamp(volID); err != nil {
						slog.Error("SetVolumeDeletionTimestamp() failed; 次の周期で再試行します", "volId", volID, "err", err)
					} else {
						slog.Debug("放置ボリュームにDeletionTimeStampを設定", "volId", volID)
					}
				}
				continue
			}
		}

		slog.Debug("ボリュームの情報", "volId", volID, "volName", vol.Metadata.Name, "volStatus", *vol.Status.Status)
		switch vol.Status.StatusCode {
		case db.VOLUME_PENDING:
			slog.Debug("待ち状態のボリュームを処理", "volId", volID)
			c.provisionPendingVolume(vol, volumeProvisionOps{
				setStatus:        c.db.UpdateVolumeStatus,
				setStatusMessage: c.db.UpdateVolumeStatusMessage,
				create: func(id string) error {
					_, err := c.marmot.CreateNewVolume(id)
					return err
				},
				configureISCSI: c.marmot.ConfigureISCSIForVolumeByID,
			})
		case db.VOLUME_PROVISIONING:
			slog.Debug("プロビジョニング中のボリュームを処理", "volId", volID)

		case db.VOLUME_ERROR:
			slog.Debug("エラー状態のボリュームを処理", "volId", volID)
			// エラー状態のボリュームを処理するコードをここに追加
		case db.VOLUME_UNAVAILABLE:
			slog.Debug("実体欠損状態のボリュームを処理", "volId", volID)
		case db.VOLUME_AVAILABLE:
			slog.Debug("利用可能なボリュームを処理", "volId", volID)
			if err := marmotd.CheckVolumeBackingStore(vol); err != nil {
				slog.Warn("AVAILABLE ボリュームの実体が見つからないため UNAVAILABLE に更新", "volId", volID, "err", err)
				if dbErr := c.db.UpdateVolumeStatusMessage(volID, db.VOLUME_UNAVAILABLE, err.Error()); dbErr != nil {
					slog.Error("UpdateVolumeStatusMessage() failed; 次の周期で再試行します", "volId", volID, "status", db.VOLUME_UNAVAILABLE, "err", dbErr)
				}
			}
		default:
			slog.Warn("不明なステータスのボリュームをスキップ", "volId", volID, "status", *vol.Status.Status)
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
}

// provisionPendingVolume は PENDING のボリュームを作成する。
// PROVISIONING への更新は実体作成の前提であり、失敗した場合は実体を作成せず、次の周期で再試行する。
// 実体の作成後に AVAILABLE を記録できない場合は、実体を削除せず、記録を再試行する。
func (c *volumeController) provisionPendingVolume(vol api.Volume, ops volumeProvisionOps) {
	volID := api.VolumeID(vol)

	if err := ops.setStatus(volID, db.VOLUME_PROVISIONING); err != nil {
		slog.Error("UpdateVolumeStatus() failed; 実体を作成せず、次の周期で再試行します", "volId", volID, "status", db.VOLUME_PROVISIONING, "err", err)
		return
	}
	if err := ops.create(volID); err != nil {
		slog.Error("CreateNewVolume()", "err", err)
		c.recordVolumeError(volID, err, ops)
		return
	}

	isISCSIVolume := vol.Spec.Type != nil && *vol.Spec.Type == "lvm" &&
		vol.Spec.Kind != nil && *vol.Spec.Kind == "data" &&
		vol.Spec.Iscsi != nil && *vol.Spec.Iscsi
	if isISCSIVolume {
		if err := ops.configureISCSI(volID); err != nil {
			slog.Error("ConfigureISCSIForVolumeByID()", "err", err, "volId", volID)
			c.recordVolumeError(volID, err, ops)
			return
		}
	}

	if err := retryVolumeStatusWrite(volumeStatusWriteAttempts, volumeStatusWriteRetryInterval, func() error {
		return ops.setStatus(volID, db.VOLUME_AVAILABLE)
	}); err != nil {
		slog.Error("UpdateVolumeStatus() failed; 作成済みの実体は削除せず、AVAILABLE の記録を次の周期以降で再試行します", "volId", volID, "err", err)
		if c.unrecordedVolumes == nil {
			c.unrecordedVolumes = make(map[string]struct{})
		}
		c.unrecordedVolumes[volID] = struct{}{}
	}
}

// recordVolumeError は作成失敗を ERROR として記録する。記録できない場合は PROVISIONING のまま古いボリュームとして回収される。
func (c *volumeController) recordVolumeError(volID string, cause error, ops volumeProvisionOps) {
	if err := ops.setStatusMessage(volID, db.VOLUME_ERROR, cause.Error()); err != nil {
		slog.Error("UpdateVolumeStatusMessage() failed", "volId", volID, "status", db.VOLUME_ERROR, "err", err)
	}
}

// retryUnrecordedVolume は、AVAILABLE を記録できていないボリュームの記録を再試行する。
// true を返した場合、そのボリュームは古いボリュームの自動削除などの以降の処理を行わない。
func (c *volumeController) retryUnrecordedVolume(vol api.Volume, setStatus func(id string, status int) error) bool {
	volID := api.VolumeID(vol)
	if _, ok := c.unrecordedVolumes[volID]; !ok {
		return false
	}
	// 削除が要求された、または他の状態に変わった場合は、状態を上書きしない
	if vol.Status == nil || vol.Status.StatusCode != db.VOLUME_PROVISIONING || vol.Status.DeletionTimeStamp != nil {
		delete(c.unrecordedVolumes, volID)
		return false
	}
	if err := setStatus(volID, db.VOLUME_AVAILABLE); err != nil {
		slog.Error("UpdateVolumeStatus() failed; AVAILABLE の記録を次の周期で再試行します", "volId", volID, "err", err)
		return true
	}
	delete(c.unrecordedVolumes, volID)
	return true
}

// forgetMissingUnrecordedVolumes は、すでに存在しないボリュームを再試行の対象から外す。
func (c *volumeController) forgetMissingUnrecordedVolumes(vols []api.Volume) {
	if len(c.unrecordedVolumes) == 0 {
		return
	}
	existing := make(map[string]struct{}, len(vols))
	for _, vol := range vols {
		existing[api.VolumeID(vol)] = struct{}{}
	}
	for volID := range c.unrecordedVolumes {
		if _, ok := existing[volID]; !ok {
			delete(c.unrecordedVolumes, volID)
		}
	}
}

// retryVolumeStatusWrite は write が成功するまで、最大 attempts 回、interval の間隔で実行する。
func retryVolumeStatusWrite(attempts int, interval time.Duration, write func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(interval)
		}
		if err = write(); err == nil {
			return nil
		}
	}
	return err
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
