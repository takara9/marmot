package controller_vol

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
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

		slog.Debug("ボリュームの情報", "volId", vol.Id, "volName", *vol.Metadata.Name, "volStatus", *vol.Status.Status)
		switch *vol.Status.Status {
		case db.VOLUME_PROVISIONING:
			slog.Debug("プロビジョニング中のボリュームを処理", "volId", vol.Id)
			v, err := c.marmot.CreateNewVolume(vol.Id)
			if err != nil {
				slog.Error("CreateNewVolume()", "err", err)
				vol.Status.Status = util.IntPtrInt(db.VOLUME_ERROR)
				if err2 := c.db.UpdateVolume(vol.Id, vol); err2 != nil {
					slog.Error("UpdateVolumeStatus() failed", "err", err2, "volId", vol.Id)
				}
			} else {
				slog.Info("ボリュームのプロビジョニング完了", "volId", v.Id, "volPath", *v.Spec.Path)
			}
		case db.VOLUME_DELETING:
			slog.Debug("削除中のボリュームを処理", "volId", vol.Id)
			err := c.marmot.RemoveVolume(vol.Id)
			if err != nil {
				slog.Error("RemoveVolume()", "err", err)
			} else {
				slog.Info("ボリュームの削除完了", "volId", vol.Id)
				// ボリュームの状態をDELETEDに更新
				vol.Status.Status = util.IntPtrInt(db.VOLUME_DELETED)
				if err2 := c.db.UpdateVolume(vol.Id, vol); err2 != nil {
					slog.Error("UpdateVolumeStatus() failed", "err", err2, "volId", vol.Id)
				}
			}
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
