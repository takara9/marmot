package controller

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
	SERVER_CONTROLLER_INTERVAL = 5 * time.Second
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
	ticker := time.NewTicker(SERVER_CONTROLLER_INTERVAL)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.serverControllerLoop()
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) serverControllerLoop() {
	slog.Info("サーバーコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	// サーバースペック情報の取得
	slog.Debug("サーバースペック情報取得", "", "")
	serverSpec, err := c.marmot.GetServersManage()
	if err != nil {
		slog.Error("GetServersManage()", "err", err)
		return
	}

	for _, spec := range serverSpec {
		// 取得したサーバースペック情報の表示とプロビジョニング中サーバーの検出
		//jsonByte, err := json.MarshalIndent(spec, "", "  ")
		//if err != nil {
		//	slog.Error("json.MarshalIndent()", "err", err)
		//	continue
		//}
		//fmt.Println(string(jsonByte))

		// 削除のタイムスタンプが一定時間以上経過しているかをチェックして、削除処理を実行する
		if spec.Status != nil && spec.Status.DeletionTimeStamp != nil {
			deletionTime := *spec.Status.DeletionTimeStamp
			if time.Since(deletionTime) > 10*time.Second {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているサーバー検出", "SERVER", spec.Id)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_DELETING, "")
			}
		}

		// サーバーの状態に応じた処理を実行する
		switch spec.Status.StatusCode {
		case db.SERVER_PENDING:
			slog.Debug("生成待ち状態のサーバー検出", "SERVER", spec.Id)
			c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_PROVISIONING, "")
			if _, err := c.marmot.CreateServerManage(spec.Id); err != nil {
				slog.Error("CreateServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーのプロビジョニングに失敗: %v", err)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_ERROR, msg)
				continue
			}
			c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_RUNNING, "")
		case db.SERVER_RUNNING:
			slog.Debug("稼働中のサーバー検出", "SERVER", spec.Id)
		case db.SERVER_STOPPED:
			slog.Debug("停止中のサーバー検出", "SERVER", spec.Id)
		case db.SERVER_ERROR:
			slog.Debug("エラー状態のサーバー検出", "SERVER", spec.Id)
		case db.SERVER_DELETING:
			slog.Debug("削除中のサーバー検出", "SERVER", spec.Id)

			// 仮想マシンの削除処理の実行
			if err := c.marmot.DeleteServerByIdManage(spec.Id); err != nil {
				slog.Error("DeleteServerById()", "err", err)
				msg := fmt.Sprintf("サーバーの削除に失敗: %v", err)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_ERROR, msg)
			}

			// IPアドレス開放処理の実行
			if spec.Spec.NetworkInterface != nil {
				for _, nic := range *spec.Spec.NetworkInterface {
					fmt.Println("NIC=========================================")
					jsonbyte, err := json.MarshalIndent(nic, "", "  ")
					if err != nil {
						slog.Error("json.MarshalIndent()", "err", err)
						continue
					}
					fmt.Println(string(jsonbyte))
					fmt.Println("=========================================")

					if nic.IpNetworkId != nil && nic.Address != nil {
						if err := c.marmot.Db.ReleaseIP(nic.Networkid, *nic.IpNetworkId, *nic.Address); err != nil {
							slog.Error("ReleaseIP()", "err", err)
							continue
						}
					}

					// 内部DNSからエントリーを削除する
					if spec.Metadata != nil && spec.Metadata.Name != nil {
						if err := c.marmot.Db.DeleteDnsEntryByName(*spec.Metadata.Name, nic.Networkname); err != nil {
							slog.Error("DeleteDnsEntryByName()", "err", err)
							continue
						}
					}
				}
			}

			// データベースから削除する
			if err := c.marmot.Db.DeleteServerById(spec.Id); err != nil {
				slog.Error("DeleteServerById()", "err", err)
				msg := fmt.Sprintf("サーバーのデータベースからの削除に失敗: %v", err)
				c.marmot.Db.UpdateServerStatus(spec.Id, db.SERVER_ERROR, msg)
			}

		case db.SERVER_PROVISIONING:
			slog.Debug("プロビジョニング中のサーバー検出", "SERVER", spec.Id)

		default:
			slog.Warn("不明な状態のサーバー検出", "SERVER", spec.Id, "STATUS", *spec.Status.Status)
		}
	}

	// ワークキューから処理を取り出して、処理を実行する

}
