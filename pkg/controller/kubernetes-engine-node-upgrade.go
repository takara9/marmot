package controller

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

// kubernetesEngineNodeUpgradeReadyPollInterval/Timeout/kubernetesEngineSleep はテストから
// 差し替え可能にするためのパッケージ変数。
var (
	kubernetesEngineNodeUpgradeReadyPollInterval = 5 * time.Second
	kubernetesEngineNodeUpgradeReadyTimeout      = 5 * time.Minute
	kubernetesEngineSleep                        = time.Sleep
)

// selectKubernetesEngineNodeForUpgrade は、DeletionTimeStamp未設定のアクティブノードのうち、
// kubernetesEngineNodeLabelKubeletVersionラベルが現在のresolvedバージョンと一致しないものを、
// 命名規則の番号(index)が最小のものから1台選ぶ。全ノードが最新であれば nil, nil を返す。
func selectKubernetesEngineNodeForUpgrade(database *db.Database, ke api.KubernetesEngine) (*api.Server, error) {
	if ke.Status == nil || ke.Status.ResolvedKubernetesVersion == nil {
		return nil, fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	resolvedVersion := strings.TrimSpace(*ke.Status.ResolvedKubernetesVersion)

	servers, err := findKubernetesEngineNodeServers(database, ke)
	if err != nil {
		return nil, err
	}
	active := make([]api.Server, 0, len(servers))
	for _, server := range servers {
		if server.Status == nil || server.Status.DeletionTimeStamp == nil {
			active = append(active, server)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		indexI, errI := kubernetesEngineNodeIndex(active[i].Metadata.Name)
		indexJ, errJ := kubernetesEngineNodeIndex(active[j].Metadata.Name)
		if errI != nil || errJ != nil {
			return active[i].Metadata.Name < active[j].Metadata.Name
		}
		return indexI < indexJ
	})
	for _, server := range active {
		labels := map[string]interface{}{}
		if server.Metadata.Labels != nil {
			labels = *server.Metadata.Labels
		}
		current, _ := labels[kubernetesEngineNodeLabelKubeletVersion].(string)
		if current != resolvedVersion {
			target := server
			return &target, nil
		}
	}
	return nil, nil
}

// waitForKubernetesEngineNodeReady は、対象ノードがKubernetes上でReady状態になるまで
// ポーリングする。kubernetesEngineNodeUpgradeReadyTimeoutを超えてもReadyにならない場合はエラーを返す。
func waitForKubernetesEngineNodeReady(ke api.KubernetesEngine, nodeName string) error {
	deadline := time.Now().Add(kubernetesEngineNodeUpgradeReadyTimeout)
	for {
		ready, err := queryKubernetesEngineNodes(ke, []string{nodeName})
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("node %s did not become Ready after upgrade within %s", nodeName, kubernetesEngineNodeUpgradeReadyTimeout)
		}
		kubernetesEngineSleep(kubernetesEngineNodeUpgradeReadyPollInterval)
	}
}

// UpgradeKubernetesEngineNode は、対象ノード1台をcordon→drain(4bで実装したドレイン処理を流用)→
// kubelet/kube-proxyバイナリの強制差し替え・再起動→uncordon→Ready確認まで進め、完了後に
// kubernetesEngineNodeLabelKubeletVersionラベルを現在のresolvedバージョンへ更新する。
func UpgradeKubernetesEngineNode(database *db.Database, mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine, server api.Server) error {
	if ke.Status == nil || ke.Status.ResolvedKubernetesVersion == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	nodeName := server.Metadata.Name
	if err := drainKubernetesEngineNode(ke, nodeName); err != nil {
		return err
	}
	nodeIP, err := kubernetesEngineNodeInternalIP(server, kubernetesEngineNetworkName(ke))
	if err != nil {
		return err
	}
	nodeIndex, err := kubernetesEngineNodeIndex(nodeName)
	if err != nil {
		return err
	}
	if err := configureKubernetesEngineNodeBinaries(mkeConf, ke, nodeName, nodeIP, nodeIndex, true); err != nil {
		return err
	}
	if err := uncordonKubernetesEngineNode(ke, nodeName); err != nil {
		return err
	}
	if err := waitForKubernetesEngineNodeReady(ke, nodeName); err != nil {
		return err
	}

	labels := map[string]interface{}{}
	if server.Metadata.Labels != nil {
		labels = *server.Metadata.Labels
	}
	labels[kubernetesEngineNodeLabelKubeletVersion] = strings.TrimSpace(*ke.Status.ResolvedKubernetesVersion)
	if err := database.UpdateServer(server.Metadata.Id, api.Server{Metadata: api.Metadata{Labels: &labels}}); err != nil {
		return fmt.Errorf("failed to mark node %s as upgraded: %w", nodeName, err)
	}
	return nil
}
