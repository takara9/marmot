package controller

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	SCHEDULER_CONTROLLER_INTERVAL = 5 * time.Second
)

type schedulerController struct {
	marmot   *marmotd.Marmot
	stopChan chan struct{}
	doneChan chan struct{}
	stopOnce sync.Once
}

// StartSchedulerController はスケジューラーコントローラーを起動する。
// リーダーノードのみが実際のスケジューリングを実行する。
func StartSchedulerController(node string, etcdUrl string) (*schedulerController, error) {
	var c schedulerController
	var err error

	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance for scheduler controller", "err", err)
		return nil, err
	}

	ticker := time.NewTicker(SCHEDULER_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.schedulerControllerLoop()
			case <-c.stopChan:
				slog.Info("スケジューラーコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// Stop はスケジューラーコントローラーを停止する。
func (c *schedulerController) Stop() {
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

// schedulerControllerLoop はリーダーノードで未割り当ての PENDING サーバーにノードを割り当てる。
func (c *schedulerController) schedulerControllerLoop() {
	slog.Debug("スケジューラーコントローラーの制御ループ実行", "node", c.marmot.NodeName, "time", time.Now().Format("2006-01-02 15:04:05"))

	// 全ホストステータスを取得してアクティブなホスト一覧を構築
	statuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		slog.Error("GetAllHostStatus() failed", "err", err)
		return
	}

	// リーダーでなければスケジューリングをスキップ
	if !marmotd.IsSchedulerLeader(c.marmot.NodeName, statuses) {
		slog.Debug("スケジューラーリーダーではないためスキップ", "node", c.marmot.NodeName)
		return
	}
	slog.Debug("スケジューラーリーダーとして動作中", "node", c.marmot.NodeName)

	// 全サーバーを取得して未割り当て PENDING サーバーを検出
	servers, err := c.marmot.Db.GetServers()
	if err != nil {
		slog.Error("GetServers() failed", "err", err)
		return
	}

	for _, server := range servers {
		// PENDING 状態でない場合はスキップ
		if server.Status == nil || server.Status.StatusCode != db.SERVER_PENDING {
			continue
		}
		// NodeName が既に設定済みの場合はスキップ
		if server.Metadata != nil && server.Metadata.NodeName != nil &&
			strings.TrimSpace(*server.Metadata.NodeName) != "" {
			continue
		}

		// 割り当て先ノードをスコアリングで選定
		targetNode, err := marmotd.SelectNode(statuses)
		if err != nil {
			slog.Error("SelectNode() failed", "err", err, "serverId", server.Id)
			continue
		}

		// ノードを割り当て
		if err := c.marmot.Db.AssignNodeToServer(server.Id, targetNode); err != nil {
			slog.Warn("AssignNodeToServer() failed", "err", err, "serverId", server.Id, "targetNode", targetNode)
			continue
		}
		slog.Info("サーバーにノードを割り当てました", "serverId", server.Id, "targetNode", targetNode)
	}
}
