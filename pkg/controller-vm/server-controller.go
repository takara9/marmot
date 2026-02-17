package controller_vm

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
func StartVmController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error

	// 初期化
	// marmotd との接続設定
	//Server := marmotd.NewServer(node, etcdUrl)
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
	slog.Info("コントローラーの制御ループ実行", "CONTROLLER", controllerCounter)
	controllerCounter++

	// サーバースペック情報の取得
	slog.Debug("サーバースペック情報取得", "", "")
	serverSpec, err := c.db.GetServers()
	if err != nil {
		slog.Error("GetServers()", "err", err)
		return
	}

	for _, spec := range serverSpec {
		// 取得したサーバースペック情報の表示とプロビジョニング中サーバーの検出
		json, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			slog.Error("json.MarshalIndent()", "err", err)
			continue
		}
		fmt.Println(string(json))

		// 削除のタイムスタンプが一定時間以上経過しているかをチェックして、削除処理を実行するとするのが良さそう
		if spec.Status != nil && spec.Status.DeletionTimeStamp != nil {
			deletionTime := *spec.Status.DeletionTimeStamp
			if time.Since(deletionTime) > 10*time.Second {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているサーバー検出", "SERVER", spec.Id)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_DELETING)
			}
		}

		// サーバーの状態チェックと処理
		// ここでワークキューに積むなどの処理を行う
		switch *spec.Status.Status {
		case db.SERVER_PENDING:
			slog.Debug("待ち状態のサーバー検出", "SERVER", spec.Id)
			c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_PROVISIONING)
			_, err := c.marmot.CreateServer2(spec.Id)
			if err != nil {
				slog.Error("CreateServer2()", "err", err)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_ERROR)
				continue
			}
			c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_RUNNING)
		case db.SERVER_RUNNING:
			slog.Debug("稼働中のサーバー検出", "SERVER", spec.Id)
		case db.SERVER_STOPPED:
			slog.Debug("停止中のサーバー検出", "SERVER", spec.Id)
		case db.SERVER_ERROR:
			slog.Debug("エラー状態のサーバー検出", "SERVER", spec.Id)
		case db.SERVER_DELETING:
			// 削除のタイムスタンプが一定時間以上経過しているかをチェックして、削除処理を実行するとするのが良さそう
			slog.Debug("削除中のサーバー検出", "SERVER", spec.Id)

			// 削除処理の実行
			if err := c.marmot.DeleteServerById(spec.Id); err != nil {
				slog.Error("DeleteServerById()", "err", err)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_ERROR)
				continue
			}
			c.marmot.Db.DeleteServerById(spec.Id)

		case db.SERVER_PROVISIONING:
			slog.Debug("プロビジョニング中のサーバー検出", "SERVER", spec.Id)

		default:
			slog.Warn("不明な状態のサーバー検出", "SERVER", spec.Id, "STATUS", *spec.Status.Status)
		}
	}

	// ワークキューから処理を取り出して、処理を実行する

}
