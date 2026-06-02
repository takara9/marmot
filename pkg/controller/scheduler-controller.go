package controller

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
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

	activeNodes := activeNodeNames(statuses)
	if len(activeNodes) == 0 {
		slog.Warn("スケジューリング対象のアクティブノードが見つかりません")
		return
	}

	nodeLoads := buildNodeLoads(activeNodes, servers)

	for _, server := range servers {
		// PENDING 状態でない場合はスキップ
		if server.Status == nil || server.Status.StatusCode != db.SERVER_PENDING {
			continue
		}

		assignedNode := ""
		if server.Metadata.NodeName != nil {
			assignedNode = strings.TrimSpace(*server.Metadata.NodeName)
		}

		// metadata.nodeName が指定済みの場合は存在チェックのみ行い、未知ノードなら ERROR に更新する。
		if assignedNode != "" {
			if !clusterHasNode(statuses, assignedNode) {
				msg := "metadata.nodeName=" + assignedNode + " はクラスタ内に存在しません"
				slog.Warn("存在しない nodeName が指定されたサーバーを ERROR に更新", "serverId", api.ServerID(server), "nodeName", assignedNode)
				c.marmot.Db.UpdateServerStatus(api.ServerID(server), db.SERVER_ERROR, msg)
			}
			continue
		}

		// 同一ループ内で均等化するため、ローカルの負荷カウントを使って選定する。
		targetNode, err := selectLeastLoadedNode(activeNodes, nodeLoads)
		if err != nil {
			slog.Error("selectLeastLoadedNode() failed", "err", err, "serverId", api.ServerID(server))
			continue
		}

		// ノードを割り当て
		if err := c.marmot.Db.AssignNodeToServer(api.ServerID(server), targetNode); err != nil {
			slog.Warn("AssignNodeToServer() failed", "err", err, "serverId", api.ServerID(server), "targetNode", targetNode)
			continue
		}
		nodeLoads[targetNode]++
		slog.Info("サーバーにノードを割り当てました", "serverId", api.ServerID(server), "targetNode", targetNode)
	}
}

func activeNodeNames(statuses []api.HostStatus) []string {
	active := make([]string, 0)
	cutoff := time.Now().Add(-marmotd.ActiveHostThreshold)
	for _, st := range statuses {
		if st.LastUpdated == nil || !st.LastUpdated.After(cutoff) {
			continue
		}
		if st.NodeName == nil {
			continue
		}
		nodeName := strings.TrimSpace(*st.NodeName)
		if nodeName == "" {
			continue
		}
		active = append(active, nodeName)
	}
	return active
}

func buildNodeLoads(activeNodes []string, servers []api.Server) map[string]int {
	loads := make(map[string]int, len(activeNodes))
	for _, node := range activeNodes {
		loads[node] = 0
	}

	for _, server := range servers {
		if server.Metadata.NodeName == nil {
			continue
		}
		nodeName := strings.TrimSpace(*server.Metadata.NodeName)
		if nodeName == "" {
			continue
		}
		if _, exists := loads[nodeName]; !exists {
			continue
		}
		if server.Status != nil && server.Status.StatusCode == db.SERVER_DELETING {
			continue
		}
		loads[nodeName]++
	}

	return loads
}

func selectLeastLoadedNode(activeNodes []string, nodeLoads map[string]int) (string, error) {
	if len(activeNodes) == 0 {
		return "", marmotd.ErrNoActiveHosts
	}

	candidates := append([]string(nil), activeNodes...)
	sort.Slice(candidates, func(i, j int) bool {
		loadI := nodeLoads[candidates[i]]
		loadJ := nodeLoads[candidates[j]]
		if loadI != loadJ {
			return loadI < loadJ
		}
		return candidates[i] < candidates[j]
	})

	return candidates[0], nil
}

func clusterHasNode(statuses []api.HostStatus, nodeName string) bool {
	target := strings.TrimSpace(nodeName)
	if target == "" {
		return false
	}

	for _, st := range statuses {
		if st.NodeName == nil {
			continue
		}
		if strings.TrimSpace(*st.NodeName) == target {
			return true
		}
	}

	return false
}
