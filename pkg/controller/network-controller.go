package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/virt"
)

const (
	NETWORK_CONTROLLER_INTERVAL = 5 * time.Second
)

/*
var controllerCounter uint64 = 0

type controller struct {
	db     *db.Database
	Lock   sync.Mutex
	marmot *marmotd.Marmot
}
*/

// ネットワークコントローラーの開始
// deletionDelaySeconds に 0 を渡した場合はデフォルト値 (10秒) が使用されます。
func StartNetController(node string, etcdUrl string, deletionDelaySeconds int) (*controller, error) {
	var c controller
	var err error

	if deletionDelaySeconds <= 0 {
		deletionDelaySeconds = 10
	}
	c.deletionDelay = time.Duration(deletionDelaySeconds) * time.Second

	// 初期化
	// marmotd との接続設定
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db
	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	// NetworkFabric の初期化（Geneve オーバーレイは OVN 前提で処理）
	networkFabric := networkfabric.NewOVNFabric()

	// 起動時に既存の仮想ネットワークを取得して、データベースに登録する
	if _, err := c.marmot.GetVirtualNetworksAndPutDB(); err != nil {
		slog.Error("Failed to get virtual networks and put DB", "err", err)
		return nil, err
	}

	// 定期実行の開始
	ticker := time.NewTicker(NETWORK_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.networkControllerLoop(networkFabric)
			case <-c.stopChan:
				slog.Debug("ネットワークコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) networkControllerLoop(fabric networkfabric.NetworkFabric) {
	slog.Debug("ネットワークコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	// 既存の仮想ネットワークを取得して、データベースに登録する
	if err := c.marmot.CheckVirtualNetworks(); err != nil {
		slog.Error("Failed to get virtual networks and put DB", "err", err)
		return
	}

	vnets, err := c.marmot.GetVirtualNetwork()
	if err != nil {
		slog.Error("failed to get virtual networks", "err", err)
		return
	}

	statuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		slog.Warn("failed to get host statuses; skip overlay membership reconciliation", "err", err)
	} else {
		isLeader := marmotd.IsSchedulerLeader(c.marmot.NodeName, statuses)
		currentMemberSignature := clusterMemberSignature(statuses)
		membershipChanged := currentMemberSignature != c.lastNetworkMemberSignature
		if membershipChanged {
			slog.Debug("cluster membership changed", "controllerNode", c.marmot.NodeName, "previous", c.lastNetworkMemberSignature, "current", currentMemberSignature)
			c.lastNetworkMemberSignature = currentMemberSignature
		}

		if isLeader && membershipChanged {
			if changed, reconcileErr := c.reconcileOverlayMembershipWithCluster(vnets, statuses); reconcileErr != nil {
				slog.Warn("failed to reconcile overlay membership after cluster change", "err", reconcileErr)
			} else if changed {
				refreshed, refreshErr := c.marmot.GetVirtualNetwork()
				if refreshErr != nil {
					slog.Warn("failed to refresh virtual networks after membership reconciliation", "err", refreshErr)
				} else {
					vnets = refreshed
				}
			}
		}
	}

	for _, vnet := range vnets {
		vnetID := api.VirtualNetworkID(vnet)
		if ok, assignedNode, reason := evaluateNodeAssignment(&vnet.Metadata, c.marmot.NodeName); !ok {
			objectName := vnet.Metadata.Name
			slog.Debug("別ノード割当の仮想ネットワークをスキップ", "networkId", vnetID, "networkName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		role := networkSyncRole(&vnet.Metadata)

		// 削除タイムスタンプが設定されて一定時間経過した仮想ネットワークを削除処理へ進める。
		// 依存リソースが残っている場合は DEL-PENDING で待機し、依存解消後に DELETING へ遷移する。
		// ERROR 状態でも削除要求を優先し、削除フローへ進める。
		if vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
			if err := c.distributeDeleteIntentToSameNameNetworks(vnet); err != nil {
				slog.Error("同名ネットワークへの削除意図配布に失敗", "networkId", vnetID, "err", err)
			}
			deletionTime := *vnet.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				deps, depErr := c.collectDeleteBlockingDependencies(vnet)
				if depErr != nil {
					slog.Warn("依存リソース判定に失敗したためネットワーク削除を保留", "networkId", vnetID, "err", depErr)
				} else if deps.hasAny() {
					c.marmot.Db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_DEL_PENDING, deps.statusMessage())
					vnet.Status.StatusCode = db.NETWORK_DEL_PENDING
					slog.Debug("依存リソースが残っているためネットワーク削除を待機", "networkId", vnetID, "dependents", deps.statusMessage())
				} else {
					slog.Debug("削除のタイムスタンプが一定時間以上経過し依存リソースも存在しないため削除を開始", "networkId", vnetID)
					c.marmot.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_DELETING)
					vnet.Status.StatusCode = db.NETWORK_DELETING
				}
			}
		}
		//fmt.Println("======================================================")
		//fmt.Println("仮想ネットワーク: ", "ID=", vnet.Id)
		//if strings.TrimSpace(vnet.Metadata.Name) != "" {
		//	fmt.Println("ネットワーク 名前=", vnet.Metadata.Name)
		//}
		//byte, err := json.MarshalIndent(vnet, "", "  ")
		//if err != nil {
		//	slog.Error("failed to marshal virtual network", "err", err)
		//} else {
		//	fmt.Println("仮想ネットワークのJSON情報", "json", string(byte))
		//}
		//fmt.Println("======================================================")

		if vnet.Status != nil && vnet.Status.Status != nil {
			switch vnet.Status.StatusCode {
			case db.NETWORK_PENDING:
				if role == "follower" {
					c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_WAITING)
					continue
				}
				slog.Debug("待ち状態の仮想ネットワークを処理", "networkId", vnetID)
				if err := c.ensureFollowerNetworksWaiting(vnet); err != nil {
					slog.Error("フォロワー用ネットワークエントリーの作成に失敗", "headNetworkId", vnetID, "err", err)
				}
				if err := c.reconcileHeadProvisioningNetwork(vnet, fabric); err != nil {
					slog.Error("head network provisioning failed", "networkId", vnetID, "err", err)
					c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, err.Error())
					continue
				}

			case db.NETWORK_PROVISIONING:
				slog.Debug("プロビジョニング中の仮想ネットワークを処理", "networkId", vnetID)
				if role == "follower" {
					if err := c.reconcileFollowerWaitingNetwork(vnet, fabric); err != nil {
						slog.Error("フォロワーネットワークのプロビジョニング継続に失敗", "networkId", vnetID, "err", err)
						c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ERROR)
					}
				} else {
					if err := c.reconcileHeadProvisioningNetwork(vnet, fabric); err != nil {
						slog.Error("head network provisioning resume failed", "networkId", vnetID, "err", err)
						c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, err.Error())
					}
				}

			case db.NETWORK_DELETING:
				slog.Debug("削除中の仮想ネットワークを処理", "networkId", vnetID)
				if role == "follower" {
					// フォロワー: libvirt destroy/undefine と fabric cleanup
					if err := c.ensureVirtualNetworkAbsent(vnet); err != nil {
						slog.Error("failed to delete virtual network on follower node", "err", err, "networkId", vnetID, "controllerNode", c.marmot.NodeName)
						c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, "fabric:detach-failed:"+err.Error())
						continue
					}
					// fabric cleanup（ブリッジ削除）
					if err := fabric.DeleteBridge(&vnet); err != nil {
						slog.Warn("failed to delete bridge on follower, continuing", "networkId", vnetID, "err", err)
						// ブリッジ削除失敗は WARNING レベル、DB 削除は続行
					}
					if err := c.db.DeleteVirtualNetworkById(vnetID); err != nil {
						slog.Error("failed to delete follower network object from DB", "err", err, "networkId", vnetID)
						c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ERROR)
					}
					continue
				}
				// ヘッド: libvirt destroy/undefine → fabric cleanup → DB 削除
				if err := c.marmot.DeleteVirtualNetwork(vnetID); err != nil {
					slog.Error("DeleteVirtualNetwork()", "err", err)
					c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, "libvirt:delete-failed:"+err.Error())
					continue
				}
				// fabric cleanup
				if err := fabric.DeleteBridge(&vnet); err != nil {
					slog.Warn("failed to delete bridge on head, continuing", "networkId", vnetID, "err", err)
				}
				slog.Debug("仮想ネットワークの削除成功", "networkId", vnetID)
			case db.NETWORK_DEL_PENDING:
				slog.Debug("依存リソースの削除待ち状態の仮想ネットワークを処理", "networkId", vnetID)
			case db.NETWORK_ERROR:
				slog.Debug("エラー状態の仮想ネットワークを処理", "networkId", vnetID)
				// ERROR 状態は保持する。削除要求（DeletionTimeStamp）が入った場合のみ削除意図を伝播する。
				if role != "follower" && vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
					if err := c.distributeDeleteIntentToFollowerNetworks(vnet); err != nil {
						slog.Error("failed to distribute delete intent to follower networks", "headNetworkId", vnetID, "err", err)
					}
					continue
				}

				if role == "follower" {
					if err := c.recoverFollowerErrorNetwork(vnet, fabric); err != nil {
						slog.Warn("failed to auto-recover follower network from error", "networkId", vnetID, "err", err)
						continue
					}
				} else {
					if err := c.recoverHeadErrorNetwork(vnet, fabric); err != nil {
						slog.Warn("failed to auto-recover head network from error", "networkId", vnetID, "err", err)
						continue
					}
				}

				slog.Debug("network recovered from error", "networkId", vnetID, "networkName", vnet.Metadata.Name, "role", role)

			case db.NETWORK_ACTIVE:
				slog.Debug("利用可能な仮想ネットワークを処理", "networkId", vnetID)
				if role == "follower" {
					if err := c.reconcileFollowerActiveNetwork(vnet, fabric); err != nil {
						slog.Error("failed to reconcile follower network", "err", err, "networkId", vnetID, "controllerNode", c.marmot.NodeName)
						c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ERROR)
					}
				} else {
					if err := c.ensureOverlayMeshForNetwork(fabric, vnet); err != nil {
						slog.Error("failed to reconcile head overlay mesh", "err", err, "networkId", vnetID, "controllerNode", c.marmot.NodeName)
						c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, "fabric:overlay-failed:"+err.Error())
					}
				}

			case db.NETWORK_WAITING:
				if role != "follower" {
					continue
				}
				slog.Debug("フォロワーネットワークはヘッドノード完了待ち", "networkId", vnetID)
				if err := c.reconcileFollowerWaitingNetwork(vnet, fabric); err != nil {
					slog.Error("フォロワーネットワークの同期開始に失敗", "networkId", vnetID, "err", err)
					c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ERROR)
				}

			default:
				slog.Warn("不明なステータスの仮想ネットワークをスキップ", "networkId", vnetID, "status", *vnet.Status.Status)
			}
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
}

func (c *controller) reconcileHeadProvisioningNetwork(vnet api.VirtualNetwork, fabric networkfabric.NetworkFabric) error {
	vnetID := api.VirtualNetworkID(vnet)
	if _, err := c.db.GetVirtualNetworkById(vnetID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			slog.Debug("skip reconcile for deleted head network", "networkId", vnetID)
			return nil
		}
		return fmt.Errorf("db:lookup-failed:%w", err)
	}

	c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_PROVISIONING, "fabric:ensure-bridge")
	if err := fabric.EnsureBridge(&vnet); err != nil {
		return fmt.Errorf("fabric:bridge-failed:%w", err)
	}

	c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_PROVISIONING, "libvirt:define-start")
	net, found, err := c.marmot.Virt.GetVirtualNetworkByName(vnet.Metadata.Name)
	if err != nil {
		return fmt.Errorf("libvirt:lookup-failed:%w", err)
	}
	if !found {
		if err := c.marmot.DeployVirtualNetwork(vnet); err != nil {
			return fmt.Errorf("libvirt:deploy-failed:%w", err)
		}
	} else {
		defer func() {
			_ = net.Free()
		}()
	}

	if err := c.ensureOverlayMeshForNetwork(fabric, vnet); err != nil {
		return fmt.Errorf("fabric:overlay-failed:%w", err)
	}

	c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ACTIVE)
	return nil
}

func networkSyncRole(metadata *api.Metadata) string {
	if metadata == nil || metadata.Labels == nil {
		return "head"
	}
	role := db.GetNetworkSyncRole(*metadata.Labels)
	if role == "" {
		return "head"
	}
	return role
}

func (c *controller) ensureFollowerNetworksWaiting(headNetwork api.VirtualNetwork) error {
	if strings.TrimSpace(headNetwork.Metadata.Name) == "" {
		return nil
	}

	headNode := ""
	if headNetwork.Metadata.NodeName != nil {
		headNode = strings.TrimSpace(*headNetwork.Metadata.NodeName)
	}
	if headNode == "" {
		return fmt.Errorf("head network nodeName is empty: networkId=%s", api.VirtualNetworkID(headNetwork))
	}

	nodeStatuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		return err
	}

	nodes := make(map[string]struct{}, len(nodeStatuses))
	for _, status := range nodeStatuses {
		if status.NodeName == nil {
			continue
		}
		node := strings.TrimSpace(*status.NodeName)
		if node == "" || node == headNode {
			continue
		}
		nodes[node] = struct{}{}
	}

	for followerNode := range nodes {
		newFollowerID, createErr := c.marmot.Db.MakeFollowerVirtualNetworkEntry(headNetwork, followerNode, api.VirtualNetworkID(headNetwork))
		if createErr != nil {
			slog.Error("フォロワーネットワークエントリー作成失敗", "headNetworkId", api.VirtualNetworkID(headNetwork), "followerNode", followerNode, "err", createErr)
			continue
		}
		slog.Debug("フォロワーネットワークをWAITINGで登録", "headNetworkId", api.VirtualNetworkID(headNetwork), "followerNetworkId", newFollowerID, "followerNode", followerNode)
	}

	return nil
}

func (c *controller) reconcileOverlayMembershipWithCluster(vnets []api.VirtualNetwork, statuses []api.HostStatus) (bool, error) {
	memberNodes := collectClusterMemberNodes(statuses)
	if len(memberNodes) == 0 {
		return false, nil
	}

	memberSet := make(map[string]struct{}, len(memberNodes))
	for _, node := range memberNodes {
		memberSet[node] = struct{}{}
	}

	changed := false
	for _, vnet := range vnets {
		if networkSyncRole(&vnet.Metadata) == "follower" {
			continue
		}
		if !isGeneveOverlay(vnet) {
			continue
		}
		if vnet.Status != nil && vnet.Status.StatusCode == db.NETWORK_DELETING {
			continue
		}

		headID := api.VirtualNetworkID(vnet)
		headNode := ""
		if vnet.Metadata.NodeName != nil {
			headNode = strings.TrimSpace(*vnet.Metadata.NodeName)
		}
		if headID == "" || headNode == "" {
			continue
		}

		desiredFollowers := map[string]struct{}{}
		for node := range memberSet {
			if node == headNode {
				continue
			}
			desiredFollowers[node] = struct{}{}
		}

		existingFollowers := map[string]api.VirtualNetwork{}
		for _, candidate := range vnets {
			if candidate.Metadata.Labels == nil || candidate.Metadata.NodeName == nil {
				continue
			}
			labels := *candidate.Metadata.Labels
			if db.GetNetworkSyncRole(labels) != "follower" {
				continue
			}
			if db.GetHeadNetworkID(labels) != headID {
				continue
			}
			node := strings.TrimSpace(*candidate.Metadata.NodeName)
			if node == "" {
				continue
			}
			existingFollowers[node] = candidate
		}

		for node := range desiredFollowers {
			if _, ok := existingFollowers[node]; ok {
				continue
			}
			if _, createErr := c.marmot.Db.MakeFollowerVirtualNetworkEntry(vnet, node, headID); createErr != nil {
				slog.Error("failed to create follower network entry during membership reconciliation", "headNetworkId", headID, "followerNode", node, "err", createErr)
				continue
			}
			changed = true
		}

		for node, follower := range existingFollowers {
			if _, ok := desiredFollowers[node]; ok {
				continue
			}
			if follower.Status != nil && follower.Status.DeletionTimeStamp != nil {
				continue
			}
			if err := c.marmot.Db.SetDeleteTimestampVirtualNetwork(api.VirtualNetworkID(follower)); err != nil {
				slog.Error("failed to set deletion timestamp for stale follower network", "headNetworkId", headID, "followerNetworkId", api.VirtualNetworkID(follower), "followerNode", node, "err", err)
				continue
			}
			changed = true
		}
	}

	return changed, nil
}

func (c *controller) distributeDeleteIntentToFollowerNetworks(headNetwork api.VirtualNetwork) error {
	if headNetwork.Metadata.Labels == nil {
		return nil
	}
	labels := *headNetwork.Metadata.Labels
	if db.GetNetworkSyncRole(labels) == "follower" {
		return nil
	}

	networks, err := c.marmot.Db.GetVirtualNetworks()
	if err != nil {
		return err
	}

	for _, network := range networks {
		if api.VirtualNetworkID(network) == api.VirtualNetworkID(headNetwork) || network.Metadata.Labels == nil {
			continue
		}

		followerLabels := *network.Metadata.Labels
		if db.GetNetworkSyncRole(followerLabels) != "follower" {
			continue
		}
		if db.GetHeadNetworkID(followerLabels) != api.VirtualNetworkID(headNetwork) {
			continue
		}

		if network.Status != nil && network.Status.DeletionTimeStamp != nil {
			continue
		}

		if err := c.marmot.Db.SetDeleteTimestampVirtualNetwork(api.VirtualNetworkID(network)); err != nil {
			slog.Error("failed to set deletion timestamp to follower network", "headNetworkId", api.VirtualNetworkID(headNetwork), "followerNetworkId", api.VirtualNetworkID(network), "err", err)
			continue
		}

		slog.Debug("head network error detected: delete intent distributed to follower", "headNetworkId", api.VirtualNetworkID(headNetwork), "followerNetworkId", api.VirtualNetworkID(network))
	}

	return nil
}

func (c *controller) distributeDeleteIntentToSameNameNetworks(sourceNetwork api.VirtualNetwork) error {
	if strings.TrimSpace(sourceNetwork.Metadata.Name) == "" {
		return nil
	}
	targetName := strings.TrimSpace(sourceNetwork.Metadata.Name)
	if targetName == "" {
		return nil
	}

	networks, err := c.marmot.Db.GetVirtualNetworks()
	if err != nil {
		return err
	}

	for _, network := range networks {
		if api.VirtualNetworkID(network) == api.VirtualNetworkID(sourceNetwork) {
			continue
		}
		if strings.TrimSpace(network.Metadata.Name) == "" {
			continue
		}
		if strings.TrimSpace(network.Metadata.Name) != targetName {
			continue
		}
		if network.Status != nil && network.Status.DeletionTimeStamp != nil {
			continue
		}

		if err := c.marmot.Db.SetDeleteTimestampVirtualNetwork(api.VirtualNetworkID(network)); err != nil {
			slog.Error("同名ネットワークへの削除タイムスタンプ設定に失敗", "sourceNetworkId", api.VirtualNetworkID(sourceNetwork), "targetNetworkId", api.VirtualNetworkID(network), "networkName", targetName, "err", err)
			continue
		}

		slog.Debug("同名ネットワークへ削除意図を配布", "sourceNetworkId", api.VirtualNetworkID(sourceNetwork), "targetNetworkId", api.VirtualNetworkID(network), "networkName", targetName)
	}

	return nil
}

func (c *controller) reconcileFollowerWaitingNetwork(waitingNetwork api.VirtualNetwork, fabric networkfabric.NetworkFabric) error {
	if waitingNetwork.Metadata.Labels == nil {
		return fmt.Errorf("labels are required for waiting network: networkId=%s", api.VirtualNetworkID(waitingNetwork))
	}
	labels := *waitingNetwork.Metadata.Labels
	headNetworkID := db.GetHeadNetworkID(labels)
	if headNetworkID == "" {
		return fmt.Errorf("headNetworkId label is missing: networkId=%s", api.VirtualNetworkID(waitingNetwork))
	}

	headNetwork, err := c.marmot.Db.GetVirtualNetworkById(headNetworkID)
	if err != nil {
		return err
	}
	if headNetwork.Status == nil {
		return fmt.Errorf("head network status is nil: headNetworkId=%s", headNetworkID)
	}

	switch headNetwork.Status.StatusCode {
	case db.NETWORK_ACTIVE:
		c.db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(waitingNetwork), db.NETWORK_PROVISIONING)
		if err := c.ensureVirtualNetworkPresent(headNetwork); err != nil {
			return err
		}
		if err := c.ensureOverlayMeshForNetwork(fabric, waitingNetwork); err != nil {
			return err
		}
		c.db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(waitingNetwork), db.NETWORK_ACTIVE)
	case db.NETWORK_DELETING:
		c.db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(waitingNetwork), db.NETWORK_DELETING)
	case db.NETWORK_ERROR:
		return fmt.Errorf("head network is not available: headNetworkId=%s status=%s", headNetworkID, db.NetworkStatus[headNetwork.Status.StatusCode])
	default:
		// WAITING を維持する。
	}

	return nil
}

func (c *controller) reconcileFollowerActiveNetwork(followerNetwork api.VirtualNetwork, fabric networkfabric.NetworkFabric) error {
	if followerNetwork.Metadata.Labels == nil {
		return fmt.Errorf("labels are required for follower network: networkId=%s", api.VirtualNetworkID(followerNetwork))
	}
	labels := *followerNetwork.Metadata.Labels
	headNetworkID := db.GetHeadNetworkID(labels)
	if headNetworkID == "" {
		return fmt.Errorf("headNetworkId label is missing: networkId=%s", api.VirtualNetworkID(followerNetwork))
	}

	headNetwork, err := c.marmot.Db.GetVirtualNetworkById(headNetworkID)
	if err != nil {
		return err
	}
	if headNetwork.Status == nil {
		return fmt.Errorf("head network status is nil: headNetworkId=%s", headNetworkID)
	}

	switch headNetwork.Status.StatusCode {
	case db.NETWORK_ACTIVE:
		if err := c.ensureVirtualNetworkPresent(followerNetwork); err != nil {
			return err
		}
		return c.ensureOverlayMeshForNetwork(fabric, followerNetwork)
	case db.NETWORK_DELETING:
		c.db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(followerNetwork), db.NETWORK_DELETING)
		return nil
	default:
		return nil
	}
}

func (c *controller) recoverHeadErrorNetwork(vnet api.VirtualNetwork, fabric networkfabric.NetworkFabric) error {
	vnetID := api.VirtualNetworkID(vnet)
	c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_PROVISIONING, "recovery:head-reconcile")
	if err := c.reconcileHeadProvisioningNetwork(vnet, fabric); err != nil {
		c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, err.Error())
		return err
	}
	return nil
}

func (c *controller) recoverFollowerErrorNetwork(vnet api.VirtualNetwork, fabric networkfabric.NetworkFabric) error {
	vnetID := api.VirtualNetworkID(vnet)
	if vnet.Metadata.Labels == nil {
		return fmt.Errorf("labels are required for follower recovery: networkId=%s", vnetID)
	}
	labels := *vnet.Metadata.Labels
	headNetworkID := db.GetHeadNetworkID(labels)
	if headNetworkID == "" {
		return fmt.Errorf("headNetworkId label is missing for follower recovery: networkId=%s", vnetID)
	}

	headNetwork, err := c.marmot.Db.GetVirtualNetworkById(headNetworkID)
	if err != nil {
		return err
	}
	if headNetwork.Status == nil {
		return fmt.Errorf("head network status is nil: headNetworkId=%s", headNetworkID)
	}

	switch headNetwork.Status.StatusCode {
	case db.NETWORK_ACTIVE:
		c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_PROVISIONING, "recovery:follower-reconcile")
		if err := c.ensureVirtualNetworkPresent(vnet); err != nil {
			c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, err.Error())
			return err
		}
		if err := c.ensureOverlayMeshForNetwork(fabric, vnet); err != nil {
			c.db.UpdateVirtualNetworkStatusWithMessage(vnetID, db.NETWORK_ERROR, "fabric:overlay-failed:"+err.Error())
			return err
		}
		c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ACTIVE)
		return nil
	case db.NETWORK_DELETING:
		c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_DELETING)
		return nil
	case db.NETWORK_PENDING, db.NETWORK_PROVISIONING, db.NETWORK_WAITING:
		c.db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_WAITING)
		return nil
	case db.NETWORK_ERROR:
		return fmt.Errorf("head network is still in error: headNetworkId=%s", headNetworkID)
	default:
		return fmt.Errorf("unsupported head network status for follower recovery: headNetworkId=%s status=%d", headNetworkID, headNetwork.Status.StatusCode)
	}
}

func (c *controller) ensureVirtualNetworkPresent(vnet api.VirtualNetwork) error {
	if strings.TrimSpace(vnet.Metadata.Name) == "" {
		return fmt.Errorf("network metadata.name is required: networkId=%s", api.VirtualNetworkID(vnet))
	}

	if _, found, err := c.marmot.Virt.GetVirtualNetworkByName(vnet.Metadata.Name); err == nil && found {
		return nil
	}

	xml, err := virt.CreateVirtualNetworkXML(vnet)
	if err != nil {
		return err
	}

	if err := c.marmot.Virt.DefineAndStartVirtualNetwork(*xml); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil
		}
		return err
	}

	return nil
}

// ensureVirtualNetworkAbsent はフォロワーノードで libvirt ネットワーク実体のみを削除する。
// DB・IPネットワーク削除はヘッドノードの DeleteVirtualNetwork が担うため、ここでは行わない。
func (c *controller) ensureVirtualNetworkAbsent(vnet api.VirtualNetwork) error {
	if strings.TrimSpace(vnet.Metadata.Name) == "" {
		return fmt.Errorf("network metadata.name is required: networkId=%s", api.VirtualNetworkID(vnet))
	}

	_, found, err := c.marmot.Virt.GetVirtualNetworkByName(vnet.Metadata.Name)
	if err != nil || !found {
		// 既に存在しない場合は何もしない
		return nil
	}

	if err := c.marmot.Virt.DeleteVirtualNetwork(vnet.Metadata.Name); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return err
	}

	slog.Debug("フォロワーノードで仮想ネットワーク実体を削除", "networkId", api.VirtualNetworkID(vnet), "networkName", vnet.Metadata.Name, "controllerNode", c.marmot.NodeName)
	return nil
}

type networkDeleteDependencies struct {
	servers       []string
	loadBalancers []string
	gateways      []string
	vpnGateways   []string
}

func (d networkDeleteDependencies) hasAny() bool {
	return len(d.servers)+len(d.loadBalancers)+len(d.gateways)+len(d.vpnGateways) > 0
}

func (d networkDeleteDependencies) statusMessage() string {
	return fmt.Sprintf(
		"deletion:blocked-by-dependents servers=%d loadBalancers=%d gateways=%d vpnGateways=%d",
		len(d.servers),
		len(d.loadBalancers),
		len(d.gateways),
		len(d.vpnGateways),
	)
}

func (c *controller) collectDeleteBlockingDependencies(vnet api.VirtualNetwork) (networkDeleteDependencies, error) {
	networkName := strings.TrimSpace(vnet.Metadata.Name)
	networkID := strings.TrimSpace(api.VirtualNetworkID(vnet))

	servers, err := c.db.GetServers()
	if err != nil {
		return networkDeleteDependencies{}, err
	}
	loadBalancers, err := c.db.GetLoadBalancers()
	if err != nil {
		return networkDeleteDependencies{}, err
	}
	gateways, err := c.db.GetGateways()
	if err != nil {
		return networkDeleteDependencies{}, err
	}
	vpnGateways, err := c.db.GetVpnGateways()
	if err != nil {
		return networkDeleteDependencies{}, err
	}

	return collectNetworkDeleteDependencies(networkName, networkID, servers, loadBalancers, gateways, vpnGateways), nil
}

func collectNetworkDeleteDependencies(
	networkName string,
	networkID string,
	servers []api.Server,
	loadBalancers []api.ApplicationLoadBalancer,
	gateways []api.Gateway,
	vpnGateways []api.VpnGateway,
) networkDeleteDependencies {
	deps := networkDeleteDependencies{}
	trimmedName := strings.TrimSpace(networkName)
	trimmedID := strings.TrimSpace(networkID)

	for _, server := range servers {
		if server.Spec.NetworkInterface == nil {
			continue
		}
		for _, nic := range *server.Spec.NetworkInterface {
			if !matchesNetworkRef(trimmedName, trimmedID, nic.Networkname, nic.Networkid) {
				continue
			}
			serverName := strings.TrimSpace(server.Metadata.Name)
			if serverName == "" {
				serverName = api.ServerID(server)
			}
			deps.servers = append(deps.servers, serverName)
			break
		}
	}

	for _, loadBalancer := range loadBalancers {
		if !matchesNetworkName(trimmedName, loadBalancer.Spec.InternalVirtualNetwork) {
			continue
		}
		deps.loadBalancers = append(deps.loadBalancers, strings.TrimSpace(loadBalancer.Metadata.Name))
	}

	for _, gateway := range gateways {
		if !matchesNetworkName(trimmedName, gateway.Spec.InternalVirtualNetwork) {
			continue
		}
		deps.gateways = append(deps.gateways, strings.TrimSpace(gateway.Metadata.Name))
	}

	for _, vpnGateway := range vpnGateways {
		if !matchesNetworkName(trimmedName, vpnGateway.Spec.InternalVirtualNetwork) {
			continue
		}
		deps.vpnGateways = append(deps.vpnGateways, strings.TrimSpace(vpnGateway.Metadata.Name))
	}

	return deps
}

func matchesNetworkRef(targetName string, targetID string, candidateName string, candidateID string) bool {
	if targetID != "" && strings.TrimSpace(candidateID) == targetID {
		return true
	}
	if targetName != "" && strings.TrimSpace(candidateName) == targetName {
		return true
	}
	return false
}

func matchesNetworkName(targetName string, candidateName string) bool {
	if targetName == "" {
		return false
	}
	return strings.TrimSpace(candidateName) == targetName
}

func isGeneveOverlay(vnet api.VirtualNetwork) bool {
	if vnet.Spec.OverlayMode == nil {
		return false
	}
	return strings.EqualFold(string(*vnet.Spec.OverlayMode), string(api.Geneve))
}

func (c *controller) ensureOverlayMeshForNetwork(fabric networkfabric.NetworkFabric, vnet api.VirtualNetwork) error {
	if !isGeneveOverlay(vnet) {
		return nil
	}

	if err := fabric.EnsureBridge(&vnet); err != nil {
		return fmt.Errorf("ensure bridge failed: %w", err)
	}

	peers, err := c.resolveGenevePeerIPs(vnet)
	if err != nil {
		return err
	}

	if err := fabric.EnsureOverlayMesh(&vnet, peers); err != nil {
		return fmt.Errorf("ensure overlay mesh failed: %w", err)
	}

	if err := fabric.PruneOverlayMesh(&vnet, peers); err != nil {
		return fmt.Errorf("prune overlay mesh failed: %w", err)
	}

	return nil
}

func (c *controller) resolveGenevePeerIPs(vnet api.VirtualNetwork) ([]string, error) {
	if strings.TrimSpace(vnet.Metadata.Name) == "" {
		return nil, fmt.Errorf("network metadata.name is required: networkId=%s", api.VirtualNetworkID(vnet))
	}

	targetName := strings.TrimSpace(vnet.Metadata.Name)
	statuses, err := c.marmot.Db.GetAllHostStatus()
	if err != nil {
		return nil, err
	}
	ipByNode := map[string]string{}
	for _, st := range statuses {
		if st.NodeName == nil || st.IpAddress == nil {
			continue
		}
		node := strings.TrimSpace(*st.NodeName)
		ip := strings.TrimSpace(*st.IpAddress)
		if node == "" || ip == "" {
			continue
		}
		ipByNode[node] = ip
	}

	networks, err := c.marmot.Db.GetVirtualNetworks()
	if err != nil {
		return nil, err
	}

	selfNode := strings.TrimSpace(c.marmot.NodeName)
	peerSet := map[string]struct{}{}
	for _, n := range networks {
		if strings.TrimSpace(n.Metadata.Name) != targetName {
			continue
		}
		if n.Metadata.NodeName == nil {
			continue
		}
		if n.Status != nil && n.Status.StatusCode == db.NETWORK_DELETING {
			continue
		}
		node := strings.TrimSpace(*n.Metadata.NodeName)
		if node == "" || node == selfNode {
			continue
		}
		ip, ok := ipByNode[node]
		if !ok || strings.TrimSpace(ip) == "" {
			continue
		}
		peerSet[strings.TrimSpace(ip)] = struct{}{}
	}

	peers := make([]string, 0, len(peerSet))
	for ip := range peerSet {
		peers = append(peers, ip)
	}
	sort.Strings(peers)
	return peers, nil
}

func collectClusterMemberNodes(statuses []api.HostStatus) []string {
	memberSet := map[string]struct{}{}
	for _, st := range statuses {
		if st.NodeName == nil {
			continue
		}
		node := strings.TrimSpace(*st.NodeName)
		if node == "" {
			continue
		}
		memberSet[node] = struct{}{}
	}

	members := make([]string, 0, len(memberSet))
	for node := range memberSet {
		members = append(members, node)
	}
	sort.Strings(members)
	return members
}

func clusterMemberSignature(statuses []api.HostStatus) string {
	return strings.Join(collectClusterMemberNodes(statuses), ",")
}
