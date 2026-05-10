package controller

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	SERVER_CONTROLLER_INTERVAL = 5 * time.Second
)

var controllerCounter uint64 = 0

type controller struct {
	db            *db.Database
	Lock          sync.Mutex
	marmot        *marmotd.Marmot
	deletionDelay time.Duration // DeletionTimestamp 検知から削除実行までの待機時間
	stopChan      chan struct{}
	doneChan      chan struct{}
	stopOnce      sync.Once
}

// VMコントローラーの開始
// deletionDelaySeconds に 0 を渡した場合はデフォルト値 (10秒) が使用されます。
func StartVmController(node string, etcdUrl string, deletionDelaySeconds int) (*controller, error) {
	var c controller
	var err error

	if deletionDelaySeconds <= 0 {
		deletionDelaySeconds = 10
	}
	c.deletionDelay = time.Duration(deletionDelaySeconds) * time.Second

	// 初期化
	// marmotd との接続設定
	//Server := marmotd.NewServer(node, etcdUrl)
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db // 正しくないけど
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	// 定期実行の開始
	ticker := time.NewTicker(SERVER_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.serverControllerLoop()
			case <-c.stopChan:
				slog.Info("サーバーコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// Stop はコントローラーの定期処理を停止し、終了を待機する。
func (c *controller) Stop() {
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

// コントローラーの制御ループ
func (c *controller) serverControllerLoop() {
	slog.Debug("サーバーコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	clusterHasNodes := true
	statuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		slog.Warn("GetAllHostStatus() failed; クラスタノード存在確認をスキップ", "err", err)
	} else {
		clusterHasNodes = clusterHasAnyNode(statuses)
	}

	// サーバースペック情報の取得
	slog.Debug("サーバースペック情報取得", "", "")
	serverSpec, err := c.marmot.GetServersManage()
	if err != nil {
		slog.Error("GetServersManage()", "err", err)
		return
	}

	for _, spec := range serverSpec {
		// 削除のタイムスタンプが一定時間以上経過しているかをチェックして、削除処理を実行する
		if spec.Status != nil && spec.Status.DeletionTimeStamp != nil {
			deletionTime := *spec.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているサーバー検出", "SERVER", api.ServerID(spec))
				c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_DELETING, "")
				spec.Status.StatusCode = db.SERVER_DELETING
				spec.Status.Status = util.StringPtr(db.ServerStatus[db.SERVER_DELETING])
			}
		}

		forceDeleteWithoutNode, bypassReason := shouldBypassNodeGateForDeletingServer(spec, statuses)

		// サーバーは必ず nodeName 割当後に処理する。
		if !forceDeleteWithoutNode && (spec.Metadata == nil || spec.Metadata.NodeName == nil || strings.TrimSpace(*spec.Metadata.NodeName) == "") {
			objectName := ""
			if spec.Metadata != nil && spec.Metadata.Name != nil {
				objectName = *spec.Metadata.Name
			}
			slog.Debug("nodeName 未割当サーバーをスキップ", "serverId", api.ServerID(spec), "serverName", objectName, "controllerNode", c.marmot.NodeName, "reason", "assigned_node_missing")
			continue
		}

		if !forceDeleteWithoutNode {
			if ok, assignedNode, reason := evaluateNodeAssignment(spec.Metadata, c.marmot.NodeName); !ok {
				objectName := ""
				if spec.Metadata != nil && spec.Metadata.Name != nil {
					objectName = *spec.Metadata.Name
				}
				slog.Debug("別ノード割当のサーバーをスキップ", "serverId", api.ServerID(spec), "serverName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
				continue
			}
		} else {
			objectName := ""
			if spec.Metadata != nil && spec.Metadata.Name != nil {
				objectName = *spec.Metadata.Name
			}
			slog.Warn("nodeName 判定をバイパスして削除を継続", "serverId", api.ServerID(spec), "serverName", objectName, "reason", bypassReason)
		}

		// 取得したサーバースペック情報の表示とプロビジョニング中サーバーの検出
		//jsonByte, err := json.MarshalIndent(spec, "", "  ")
		//if err != nil {
		//	slog.Error("json.MarshalIndent()", "err", err)
		//	continue
		//}
		//fmt.Println(string(jsonByte))

		// サーバーの状態に応じた処理を実行する
		switch spec.Status.StatusCode {
		case db.SERVER_PENDING:
			slog.Debug("生成待ち状態のサーバー検出", "SERVER", api.ServerID(spec))
			c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_PROVISIONING, "")
			if _, err := c.marmot.CreateServerManage(api.ServerID(spec)); err != nil {
				slog.Error("CreateServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーのプロビジョニングに失敗した。原因エラー: %v", err)
				c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg)
				continue
			}
			c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_RUNNING, "")
		case db.SERVER_RUNNING:
			slog.Debug("稼働中のサーバー検出", "SERVER", api.ServerID(spec))
		case db.SERVER_STOPPING:
			slog.Debug("停止要求のサーバー検出", "SERVER", api.ServerID(spec))
			if err := c.marmot.StopServerManage(api.ServerID(spec)); err != nil {
				slog.Error("StopServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーの停止に失敗: %v", err)
				c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg)
			}
		case db.SERVER_STOPPED:
			slog.Debug("停止中のサーバー検出", "SERVER", api.ServerID(spec))
		case db.SERVER_STARTING:
			slog.Debug("起動要求のサーバー検出", "SERVER", api.ServerID(spec))
			if err := c.marmot.StartServerManage(api.ServerID(spec)); err != nil {
				slog.Error("StartServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーの起動に失敗: %v", err)
				c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg)
			}
		case db.SERVER_ERROR:
			slog.Debug("エラー状態のサーバー検出", "SERVER", api.ServerID(spec))
		case db.SERVER_DELETING:
			slog.Debug("削除中のサーバー検出", "SERVER", api.ServerID(spec))

			if !clusterHasNodes {
				if spec.Spec != nil && spec.Spec.BootVolume != nil && strings.TrimSpace(spec.Spec.BootVolume.Id) != "" {
					c.marmot.Db.SetVolumeDeletionTimestamp(spec.Spec.BootVolume.Id)
				}
				if spec.Spec != nil && spec.Spec.Storage != nil {
					for _, vol := range *spec.Spec.Storage {
						if vol.Spec != nil && vol.Spec.Persistent != nil && *vol.Spec.Persistent {
							continue
						}
						if strings.TrimSpace(vol.Id) == "" {
							continue
						}
						c.marmot.Db.SetVolumeDeletionTimestamp(vol.Id)
					}
				}
				slog.Warn("クラスタノード不在のため VM 実体削除をスキップし、サーバー定義削除を継続", "serverId", api.ServerID(spec))
			} else {
				// 仮想マシンの削除処理の実行
				if err := c.marmot.DeleteServerByIdManage(api.ServerID(spec)); err != nil {
					slog.Error("DeleteServerById()", "err", err)
					msg := fmt.Sprintf("サーバーの削除に失敗: %v", err)
					c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg)
				}
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
			if err := c.marmot.Db.DeleteServerById(api.ServerID(spec)); err != nil {
				slog.Error("DeleteServerById()", "err", err)
				msg := fmt.Sprintf("サーバーのデータベースからの削除に失敗: %v", err)
				c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg)
			}

		case db.SERVER_PROVISIONING:
			slog.Debug("プロビジョニング中のサーバー検出", "SERVER", api.ServerID(spec))

		default:
			slog.Warn("不明な状態のサーバー検出", "SERVER", api.ServerID(spec), "STATUS", *spec.Status.Status)
		}
	}

	// ワークキューから処理を取り出して、処理を実行する

}

func clusterHasAnyNode(statuses []api.HostStatus) bool {
	for _, st := range statuses {
		if st.NodeName == nil {
			continue
		}
		if strings.TrimSpace(*st.NodeName) != "" {
			return true
		}
	}

	return false
}

func shouldBypassNodeGateForDeletingServer(spec api.Server, statuses []api.HostStatus) (bool, string) {
	if spec.Status == nil || spec.Status.StatusCode != db.SERVER_DELETING {
		return false, ""
	}

	if !clusterHasAnyNode(statuses) {
		return true, "cluster_nodes_empty"
	}

	if spec.Metadata == nil || spec.Metadata.NodeName == nil {
		return true, "assigned_node_missing"
	}

	assignedNode := strings.TrimSpace(*spec.Metadata.NodeName)
	if assignedNode == "" {
		return true, "assigned_node_empty"
	}

	if !clusterHasNode(statuses, assignedNode) {
		return true, "assigned_node_not_found"
	}

	return false, ""
}
