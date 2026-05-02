package controller

import (
	"log/slog"
	"time"

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

	vols, err := c.marmot.GetVolumes()
	if err != nil {
		slog.Error("failed to get volumes", "err", err)
		return
	}
	slog.Debug("取得したボリュームの数", "numVolumes", len(vols))
	for _, vol := range vols {
		if ok, assignedNode, reason := evaluateNodeAssignment(vol.Metadata, c.marmot.NodeName); !ok {
			objectName := ""
			if vol.Metadata != nil && vol.Metadata.Name != nil {
				objectName = *vol.Metadata.Name
			}
			slog.Debug("別ノード割当のボリュームをスキップ", "volumeId", vol.Id, "volumeName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			slog.Debug("ボリュームの詳細情報", "volumeId", vol.Id, "volumeName", objectName, "metadata", vol.Metadata, "status", vol.Status)
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
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているボリューム検出", "volId", vol.Id)
				c.marmot.Db.UpdateVolumeStatus(vol.Id, db.VOLUME_DELETING)
			}
		}

		// 最終更新から10分以上 ERROR / PROVISIONING が継続している場合は実体ごと自動削除
		if vol.Status != nil && vol.Status.LastUpdateTimeStamp != nil {
			isStale := time.Since(*vol.Status.LastUpdateTimeStamp) > VOLUME_STALE_TIMEOUT
			isTargetState := vol.Status.StatusCode == db.VOLUME_ERROR || vol.Status.StatusCode == db.VOLUME_PROVISIONING
			if isStale && isTargetState {
				slog.Warn("最終更新から10分以上放置されたボリュームを削除キューへ登録", "volId", vol.Id, "status", vol.Status.StatusCode, "lastUpdate", vol.Status.LastUpdateTimeStamp)
				if vol.Status.DeletionTimeStamp == nil {
					c.db.SetVolumeDeletionTimestamp(vol.Id)
					slog.Info("放置ボリュームにDeletionTimeStampを設定", "volId", vol.Id)
				}
				continue
			}
		}

		slog.Debug("ボリュームの情報", "volId", vol.Id, "volName", *vol.Metadata.Name, "volStatus", *vol.Status.Status)
		switch vol.Status.StatusCode {
		case db.VOLUME_PENDING:
			slog.Debug("待ち状態のボリュームを処理", "volId", vol.Id)
			// 待ち状態のボリュームを処理するコードをここに追加
			c.db.UpdateVolumeStatus(vol.Id, db.VOLUME_PROVISIONING)
			if _, err := c.marmot.CreateNewVolume(vol.Id); err != nil {
				slog.Error("CreateNewVolume()", "err", err)
				c.db.UpdateVolumeStatus(vol.Id, db.VOLUME_ERROR)
				continue
			}
			c.db.UpdateVolumeStatus(vol.Id, db.VOLUME_AVAILABLE)
		case db.VOLUME_PROVISIONING:
			slog.Debug("プロビジョニング中のボリュームを処理", "volId", vol.Id)

		case db.VOLUME_DELETING:
			slog.Debug("削除中のボリュームを処理", "volId", vol.Id)
			if err := c.marmot.RemoveVolume(vol.Id); err != nil {
				slog.Error("RemoveVolume()", "err", err)
				c.db.UpdateVolumeStatus(vol.Id, db.VOLUME_ERROR)
				continue
			}
			c.db.DeleteVolume(vol.Id)
			slog.Debug("ボリュームの削除成功", "volId", vol.Id)

		case db.VOLUME_ERROR:
			slog.Debug("エラー状態のボリュームを処理", "volId", vol.Id)
			// エラー状態のボリュームを処理するコードをここに追加
		case db.VOLUME_UNAVAILABLE:
			slog.Debug("実体欠損状態のボリュームを処理", "volId", vol.Id)
		case db.VOLUME_AVAILABLE:
			slog.Debug("利用可能なボリュームを処理", "volId", vol.Id)
			if err := marmotd.CheckVolumeBackingStore(vol); err != nil {
				slog.Warn("AVAILABLE ボリュームの実体が見つからないため UNAVAILABLE に更新", "volId", vol.Id, "err", err)
				c.db.UpdateVolumeStatusMessage(vol.Id, db.VOLUME_UNAVAILABLE, err.Error())
			}
		default:
			slog.Warn("不明なステータスのボリュームをスキップ", "volId", vol.Id, "status", *vol.Status.Status)
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
}
