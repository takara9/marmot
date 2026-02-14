package controller_vol

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	CONTROLLER_INTERVAL = 5 * time.Second
)

var controllerCounter uint64 = 0

type controller struct {
	db     *db.Database
	Lock   sync.Mutex
	marmot *marmotd.Marmot
}

// VMコントローラーの開始
func StartVolController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error

	// 初期化
	// marmotd との接続設定
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db // 正しくないけど

	// 定期実行の開始
	ticker := time.NewTicker(CONTROLLER_INTERVAL)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.controllerLoop()
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) controllerLoop() {
	slog.Info("ボリュームコントローラーの制御ループ実行", "CONTROLLER", controllerCounter)
	controllerCounter++
	vols, err := c.marmot.GetVolumes()
	if err != nil {
		slog.Error("failed to get volumes", "err", err)
		return
	}
	slog.Debug("取得したボリュームの数", "numVolumes", len(vols))
	for _, vol := range vols {
		byte, err := json.MarshalIndent(vol, "", "  ")
		if err != nil {
			slog.Error("failed to marshal volume", "err", err)
		} else {
			fmt.Println("ボリュームのJSON情報", "json", string(byte))
		}
		// 削除タイムスタンプが設定されて一定時間経過したボリュームのステータスをDELETINGに更新する
		if vol.Status != nil && vol.Status.DeletionTimeStamp != nil {
			deletionTime := *vol.Status.DeletionTimeStamp
			if time.Since(deletionTime) > 10*time.Second {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているボリューム検出", "volId", vol.Id)
				c.marmot.Db.UpdateVolumeStatus(vol.Id, db.VOLUME_DELETING)
			}
		}

		slog.Debug("ボリュームの情報", "volId", vol.Id, "volName", *vol.Metadata.Name, "volStatus", *vol.Status.Status)
		switch *vol.Status.Status {
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
		case db.VOLUME_AVAILABLE:
			slog.Debug("利用可能なボリュームを処理", "volId", vol.Id)
			// 利用可能なボリュームを処理するコードをここに追加
		default:
			slog.Warn("不明なステータスのボリュームをスキップ", "volId", vol.Id, "status", *vol.Status.Status)
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
}
