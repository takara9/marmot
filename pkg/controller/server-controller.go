package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

const (
	SERVER_CONTROLLER_INTERVAL = 5 * time.Second
	serverWatchMinInterval     = time.Second
	serverWatchRetryInterval   = time.Second
)

// serverController はゲストVM(Server)の状態を管理する専用コントローラー。
type serverController struct {
	db            *db.Database
	marmot        *marmotd.Marmot
	deletionDelay time.Duration // DeletionTimestamp 検知から削除実行までの待機時間
	stopChan      chan struct{}
	doneChan      chan struct{}
	stopOnce      sync.Once
}

// VMコントローラーの開始
// deletionDelaySeconds に 0 を渡した場合はデフォルト値 (10秒) が使用されます。
func StartVmController(node string, etcdUrl string, deletionDelaySeconds int) (*serverController, error) {
	var c serverController
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

	go func() {
		defer close(c.doneChan)
		ctx, cancel := context.WithCancel(context.Background())
		notifications := make(chan struct{}, 1)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			watchServerChanges(ctx, c.db.Cli.Watch, notifications, serverWatchRetryInterval)
		}()
		runServerController(c.stopChan, notifications, SERVER_CONTROLLER_INTERVAL, serverWatchMinInterval, c.serverControllerLoop)
		cancel()
		<-watchDone
		slog.Debug("サーバーコントローラー停止")
	}()
	return &c, nil
}

func watchServerChanges(ctx context.Context, watch func(context.Context, string, ...etcd.OpOption) etcd.WatchChan, notifications chan<- struct{}, retryInterval time.Duration) {
	notify := func() {
		select {
		case notifications <- struct{}{}:
		default:
		}
	}

	for ctx.Err() == nil {
		ch := watch(ctx, db.ServerPrefix+"/", etcd.WithPrefix(), etcd.WithCreatedNotify())
		for response := range ch {
			if err := response.Err(); err != nil {
				slog.Warn("Server watch failed; retrying", "err", err)
				break
			}
			if response.Created {
				slog.Info("Server watch established")
				notify()
			}
			if len(response.Events) > 0 {
				notify()
			}
		}
		if ctx.Err() != nil {
			return
		}
		slog.Warn("Server watch ended; retrying")
		timer := time.NewTimer(retryInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

func runServerController(stop <-chan struct{}, notifications <-chan struct{}, interval, minInterval time.Duration, reconcile func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastRun time.Time
	var pendingTimer *time.Timer
	var pending <-chan time.Time
	defer func() {
		if pendingTimer != nil {
			pendingTimer.Stop()
		}
	}()

	run := func() {
		reconcile()
		lastRun = time.Now()
		if pendingTimer != nil {
			pendingTimer.Stop()
			pendingTimer.Reset(minInterval)
		}
	}

	for {
		select {
		case <-stop:
			return
		default:
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
			run()
		case <-notifications:
			if pendingTimer == nil {
				delay := time.Until(lastRun.Add(minInterval))
				if delay < 0 {
					delay = 0
				}
				pendingTimer = time.NewTimer(delay)
				pending = pendingTimer.C
			}
		case <-pending:
			pendingTimer = nil
			pending = nil
			run()
		}
	}
}

// Stop はコントローラーの処理を停止し、終了を待機する。
func (c *serverController) Stop() {
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
func (c *serverController) serverControllerLoop() {
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
		if spec.Status != nil && spec.Status.StatusCode == db.SERVER_PENDING {
			assignedNode, assignErr := c.marmot.ResolveAndAssignServerNodeByStorage(api.ServerID(spec))
			if assignErr != nil {
				slog.Error("ResolveAndAssignServerNodeByStorage()", "serverId", api.ServerID(spec), "err", assignErr)
				msg := fmt.Sprintf("サーバーのノード割当解決に失敗した。原因エラー: %v", assignErr)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
				continue
			}
			if strings.TrimSpace(assignedNode) != "" {
				spec.Metadata.NodeName = util.StringPtr(assignedNode)
			}
		}

		// 削除のタイムスタンプが一定時間以上経過しているかをチェックして、削除処理を実行する
		if spec.Status != nil && spec.Status.DeletionTimeStamp != nil {
			deletionTime := *spec.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				slog.Debug("削除のタイムスタンプが一定時間以上経過しているサーバー検出", "SERVER", api.ServerID(spec))
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_DELETING, ""); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
				spec.Status.StatusCode = db.SERVER_DELETING
				spec.Status.Status = util.StringPtr(db.ServerStatus[db.SERVER_DELETING])
			}
		}

		forceDeleteWithoutNode, bypassReason := shouldBypassNodeGateForDeletingServer(spec, statuses)

		// サーバーは必ず nodeName 割当後に処理する。
		if !forceDeleteWithoutNode && (spec.Metadata.NodeName == nil || strings.TrimSpace(*spec.Metadata.NodeName) == "") {
			objectName := spec.Metadata.Name
			slog.Debug("nodeName 未割当サーバーをスキップ", "serverId", api.ServerID(spec), "serverName", objectName, "controllerNode", c.marmot.NodeName, "reason", "assigned_node_missing")
			continue
		}

		if !forceDeleteWithoutNode {
			if ok, assignedNode, reason := evaluateNodeAssignment(&spec.Metadata, c.marmot.NodeName); !ok {
				objectName := spec.Metadata.Name
				slog.Debug("別ノード割当のサーバーをスキップ", "serverId", api.ServerID(spec), "serverName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
				continue
			}
		} else {
			objectName := spec.Metadata.Name
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
			if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_PROVISIONING, ""); dbErr != nil {
				slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				continue
			}
			if _, err := c.marmot.CreateServerManage(api.ServerID(spec)); err != nil {
				slog.Error("CreateServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーのプロビジョニングに失敗した。原因エラー: %v", err)
				if isRetryableServerProvisionError(err) {
					retryMsg := fmt.Sprintf("サーバーのプロビジョニング待機中（依存関係の準備待ち）: %v", err)
					slog.Warn("CreateServerManage() failed with retryable error; keep server pending", "server", api.ServerID(spec), "err", err)
					if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_PENDING, retryMsg); dbErr != nil {
						slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
					}
					continue
				}
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
				continue
			}
			if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_RUNNING, ""); dbErr != nil {
				slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
			}
		case db.SERVER_RUNNING:
			slog.Debug("稼働中のサーバー検出", "SERVER", api.ServerID(spec))
			if err := c.marmot.SyncServerResourcesManage(api.ServerID(spec)); err != nil {
				slog.Error("SyncServerResourcesManage()", "serverId", api.ServerID(spec), "err", err)
				msg := fmt.Sprintf("サーバー設定(CPU/Memory)の反映に失敗: %v", err)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
			}
		case db.SERVER_STOPPING:
			slog.Debug("停止要求のサーバー検出", "SERVER", api.ServerID(spec))
			if err := c.marmot.StopServerManage(api.ServerID(spec)); err != nil {
				slog.Error("StopServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーの停止に失敗: %v", err)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
			}
		case db.SERVER_STOPPED:
			slog.Debug("停止中のサーバー検出", "SERVER", api.ServerID(spec))
			if err := c.marmot.ReconcileStoppedServerManage(api.ServerID(spec)); err != nil {
				slog.Error("ReconcileStoppedServerManage()", "serverId", api.ServerID(spec), "err", err)
			}
		case db.SERVER_STARTING:
			slog.Debug("起動要求のサーバー検出", "SERVER", api.ServerID(spec))
			if err := c.marmot.StartServerManage(api.ServerID(spec)); err != nil {
				slog.Error("StartServerManage()", "err", err)
				msg := fmt.Sprintf("サーバーの起動に失敗: %v", err)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
			}
		case db.SERVER_ERROR:
			slog.Debug("エラー状態のサーバー検出", "SERVER", api.ServerID(spec))
		case db.SERVER_DELETING:
			slog.Debug("削除中のサーバー検出", "SERVER", api.ServerID(spec))

			var isoCleanupErr error
			if !clusterHasNodes {
				if spec.Spec.BootVolume != nil && strings.TrimSpace(api.VolumeID(*spec.Spec.BootVolume)) != "" {
					c.marmot.Db.SetVolumeDeletionTimestamp(api.VolumeID(*spec.Spec.BootVolume))
				}
				if spec.Spec.Storage != nil {
					for _, vol := range *spec.Spec.Storage {
						if vol.Spec.Persistent != nil && *vol.Spec.Persistent {
							continue
						}
						if strings.TrimSpace(api.VolumeID(vol)) == "" {
							continue
						}
						c.marmot.Db.SetVolumeDeletionTimestamp(api.VolumeID(vol))
					}
				}
				slog.Warn("クラスタノード不在のため VM 実体削除をスキップし、サーバー定義削除を継続", "serverId", api.ServerID(spec))
			} else {
				// 仮想マシンの削除処理の実行。失敗時は VM が残っている可能性があるため、
				// サーバー定義を残して DELETING のまま次の制御ループで再試行する
				if err := c.marmot.DeleteServerByIdManage(api.ServerID(spec)); err != nil {
					slog.Error("DeleteServerById()", "serverId", api.ServerID(spec), "err", err)
					msg := fmt.Sprintf("VM の削除に失敗したため、サーバー定義を残して再試行します: %v", err)
					if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_DELETING, msg); dbErr != nil {
						slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
					}
					continue
				}
				// ドメインの削除成功または不存在を確認できたため、cloud-init ISO を削除する。
				// 失敗時はサーバー定義を残し、次の制御ループで再試行する
				isoCleanupErr = marmotd.RemoveServerCloudInitISO(api.ServerID(spec))
			}

			if isoCleanupErr != nil {
				slog.Error("RemoveServerCloudInitISO()", "serverId", api.ServerID(spec), "err", isoCleanupErr)
				msg := fmt.Sprintf("cloud-init ISO の削除に失敗したため、再試行します: %v", isoCleanupErr)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_DELETING, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
				continue
			}

			// OVN 論理ポート・内部 DNS・IP の後片付け。失敗時は Server 定義を残し、
			// DELETING のまま次の制御ループで再試行する
			if err := c.cleanupDeletedServerNetwork(spec); err != nil {
				slog.Error("cleanupDeletedServerNetwork()", "serverId", api.ServerID(spec), "err", err)
				msg := fmt.Sprintf("ネットワーク関連の後片付けに失敗したため、再試行します: %v", err)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_DELETING, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
				continue
			}

			// データベースから削除する
			if err := c.marmot.Db.DeleteServerById(api.ServerID(spec)); err != nil {
				slog.Error("DeleteServerById()", "err", err)
				msg := fmt.Sprintf("サーバーのデータベースからの削除に失敗: %v", err)
				if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
					slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
				}
			}

		case db.SERVER_PROVISIONING:
			slog.Debug("プロビジョニング中のサーバー検出", "SERVER", api.ServerID(spec))
			msg := "サーバーのプロビジョニングが中断されました。marmot サービス再起動後も自動再開されないため、サーバーを再作成してください"
			if spec.Status != nil && spec.Status.Message != nil && strings.TrimSpace(*spec.Status.Message) != "" {
				msg = *spec.Status.Message
			}
			if dbErr := c.marmot.Db.UpdateServerStatus(api.ServerID(spec), db.SERVER_ERROR, msg); dbErr != nil {
				slog.Error("UpdateServerStatus() failed", "serverId", api.ServerID(spec), "err", dbErr)
			}

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

// cleanupDeletedServerNetwork は、削除する Server の OVN 論理ポート、内部 DNS エントリー、IP を後片付けする。
// 各操作は繰り返し実行しても安全で、失敗は集約して返す。
// 解放後に他のホストへ再割り当てされた IP を再実行で解放しないよう、IP の解放は他の後片付けが
// すべて成功した後に、割り当て記録の所有者を確認して行う。
func (c *serverController) cleanupDeletedServerNetwork(spec api.Server) error {
	serverName := strings.TrimSpace(spec.Metadata.Name)
	var errs []error

	if spec.Spec.NetworkInterface != nil {
		for _, nic := range *spec.Spec.NetworkInterface {
			if jsonbyte, err := json.MarshalIndent(nic, "", "  "); err != nil {
				slog.Error("json.MarshalIndent()", "err", err)
			} else {
				debugPrintln("NIC=========================================")
				debugPrintln(string(jsonbyte))
				debugPrintln("=========================================")
			}

			// OVN論理ポートを削除する(issue #696)
			if nic.InterfaceId != nil && strings.TrimSpace(*nic.InterfaceId) != "" {
				if err := networkfabric.NewOVNFabric().DeleteGuestLogicalPort(*nic.InterfaceId); err != nil {
					errs = append(errs, fmt.Errorf("DeleteGuestLogicalPort(%s): %w", *nic.InterfaceId, err))
				}
			}

			// 内部DNSからエントリーを削除する
			if serverName != "" {
				if err := c.marmot.Db.DeleteDnsEntryByName(spec.Metadata.Name, nic.Networkname); err != nil {
					errs = append(errs, fmt.Errorf("DeleteDnsEntryByName(%s, %s): %w", spec.Metadata.Name, nic.Networkname, err))
				}
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if serverName == "" {
		slog.Warn("サーバー名が空のため、所有者を確認できず IP の解放をスキップ", "serverId", api.ServerID(spec))
		return nil
	}

	// IPアドレス開放処理の実行
	if spec.Spec.NetworkInterface != nil {
		for _, nic := range *spec.Spec.NetworkInterface {
			if nic.IpNetworkId != nil && nic.Address != nil {
				if err := c.marmot.Db.ReleaseIPIfOwned(nic.Networkid, *nic.IpNetworkId, *nic.Address, serverName); err != nil {
					errs = append(errs, fmt.Errorf("ReleaseIPIfOwned(%s): %w", *nic.Address, err))
				}
			}
		}
	}

	// 作成失敗時など NIC 情報だけでは解放しきれない残留 IP を回収する。
	if err := c.marmot.Db.ReleaseIPsByHostID(serverName); err != nil {
		errs = append(errs, fmt.Errorf("ReleaseIPsByHostID(%s): %w", serverName, err))
	}
	return errors.Join(errs...)
}

func shouldBypassNodeGateForDeletingServer(spec api.Server, statuses []api.HostStatus) (bool, string) {
	if spec.Status == nil || spec.Status.StatusCode != db.SERVER_DELETING {
		return false, ""
	}

	if !clusterHasAnyNode(statuses) {
		return true, "cluster_nodes_empty"
	}

	if spec.Metadata.NodeName == nil {
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

func isRetryableServerProvisionError(err error) bool {
	if err == nil {
		return false
	}

	// 事前に作成された Volume の実体が未作成の場合は、準備完了まで待機して再試行する。
	if errors.Is(err, marmotd.ErrStorageVolumeNotReady) {
		return true
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}

	// 依存リソース未作成時は待機して再試行する。
	if strings.Contains(msg, "network '") && strings.Contains(msg, "is not found") {
		return true
	}
	if strings.Contains(msg, "network '") && strings.Contains(msg, "is not ready for ip allocation") {
		return true
	}

	// GitHub 等からの公開鍵取得失敗は一時的障害の可能性があるため再試行する。
	if strings.Contains(msg, "failed to fetch keys from") {
		return true
	}
	if strings.Contains(msg, "unexpected http status") {
		return true
	}
	if strings.Contains(msg, "no public keys found at") {
		return true
	}

	// ノード間レプリケーション中に OS イメージ実体が未到達な場合は再試行する。
	if strings.Contains(msg, "failed to copy qcow2 volume") {
		if strings.Contains(msg, "no such file") || strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist") {
			return true
		}
	}

	// OVN論理ポート作成時の一時的障害(ovn-db再起動、接続断、タイムアウト等)は再試行する。
	if strings.Contains(msg, "failed to ensure ovn logical switch port") ||
		strings.Contains(msg, "failed to set addresses on ovn logical switch port") ||
		strings.Contains(msg, "failed to set managed external_id on ovn logical switch port") {
		if strings.Contains(msg, "connection refused") ||
			strings.Contains(msg, "connection reset") ||
			strings.Contains(msg, "timed out") ||
			strings.Contains(msg, "timeout") ||
			strings.Contains(msg, "temporar") ||
			strings.Contains(msg, "try again") ||
			strings.Contains(msg, "resource busy") {
			return true
		}
	}

	return false
}
