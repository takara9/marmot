package controller

import (
	"fmt"
	"log/slog"
	"strconv"
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

	// NetworkFabric の初期化（OVN 優先、既存 VXLAN は OVS フォールバック）
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
				slog.Info("ネットワークコントローラー停止")
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
		slog.Warn("failed to get host statuses; skip vxlan hub failover reconciliation", "err", err)
	} else if marmotd.IsSchedulerLeader(c.marmot.NodeName, statuses) {
		if err := c.reconcileVxlanHubFailovers(vnets, statuses); err != nil {
			slog.Warn("failed to reconcile vxlan hub failovers", "err", err)
		}
		refreshed, refreshErr := c.marmot.GetVirtualNetwork()
		if refreshErr != nil {
			slog.Warn("failed to refresh virtual networks after hub failover reconciliation", "err", refreshErr)
		} else {
			vnets = refreshed
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

		// 削除タイムスタンプが設定されて一定時間経過した仮想ネットワークのステータスを DELETING に更新する。
		// ERROR 状態でも削除要求を優先し、削除フローへ進める。
		if vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
			if err := c.distributeDeleteIntentToSameNameNetworks(vnet); err != nil {
				slog.Error("同名ネットワークへの削除意図配布に失敗", "networkId", vnetID, "err", err)
			}
			deletionTime := *vnet.Status.DeletionTimeStamp
			if time.Since(deletionTime) > c.deletionDelay {
				slog.Debug("削除のタイムスタンプが一定時間以上経過している仮想ネットワーク検出", "networkId", vnetID)
				c.marmot.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_DELETING)
				vnet.Status.StatusCode = db.NETWORK_DELETING
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
			case db.NETWORK_ERROR:
				slog.Debug("エラー状態の仮想ネットワークを処理", "networkId", vnetID)
				// ERROR 状態は保持する。削除要求（DeletionTimeStamp）が入った場合のみ削除意図を伝播する。
				if role != "follower" && vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
					if err := c.distributeDeleteIntentToFollowerNetworks(vnet); err != nil {
						slog.Error("failed to distribute delete intent to follower networks", "headNetworkId", vnetID, "err", err)
					}
				}
				// ERROR のまま保持する（mactl network delete 実行まで削除しない）。

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
		defer net.Free()
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

func isVxlanOverlay(vnet api.VirtualNetwork) bool {
	if vnet.Spec.OverlayMode == nil {
		return false
	}
	return strings.EqualFold(string(*vnet.Spec.OverlayMode), "vxlan")
}

func isGeneveOverlay(vnet api.VirtualNetwork) bool {
	if vnet.Spec.OverlayMode == nil {
		return false
	}
	return strings.EqualFold(string(*vnet.Spec.OverlayMode), string(api.Geneve))
}

func (c *controller) ensureOverlayMeshForNetwork(fabric networkfabric.NetworkFabric, vnet api.VirtualNetwork) error {
	if !isVxlanOverlay(vnet) && !isGeneveOverlay(vnet) {
		return nil
	}

	if err := fabric.EnsureBridge(&vnet); err != nil {
		return fmt.Errorf("ensure bridge failed: %w", err)
	}

	if isVxlanOverlay(vnet) && vxlanPeerPolicy(vnet) == api.Manual {
		slog.Debug("peerPolicy=manual: skip automatic vxlan ensure/prune", "networkId", api.VirtualNetworkID(vnet), "networkName", vnet.Metadata.Name)
		return nil
	}

	peers := []string{}
	if isVxlanOverlay(vnet) {
		resolvedPeers, err := c.resolveVxlanPeerIPs(vnet)
		if err != nil {
			return err
		}
		peers = resolvedPeers
	} else if isGeneveOverlay(vnet) {
		resolvedPeers, err := c.resolveGenevePeerIPs(vnet)
		if err != nil {
			return err
		}
		peers = resolvedPeers
	}

	if err := fabric.EnsureOverlayMesh(&vnet, peers); err != nil {
		return fmt.Errorf("ensure overlay mesh failed: %w", err)
	}

	if err := fabric.PruneOverlayMesh(&vnet, peers); err != nil {
		return fmt.Errorf("prune overlay mesh failed: %w", err)
	}

	return nil
}

func (c *controller) resolveVxlanPeerIPs(vnet api.VirtualNetwork) ([]string, error) {
	if strings.TrimSpace(vnet.Metadata.Name) == "" {
		return nil, fmt.Errorf("network metadata.name is required: networkId=%s", api.VirtualNetworkID(vnet))
	}

	targetName := strings.TrimSpace(vnet.Metadata.Name)
	if targetName == "" {
		return nil, fmt.Errorf("network metadata.name is empty: networkId=%s", api.VirtualNetworkID(vnet))
	}

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
	participantNodes := map[string]struct{}{}
	hubCandidate := vxlanHubNodeFromNetwork(vnet)
	for _, n := range networks {
		if strings.TrimSpace(n.Metadata.Name) == "" || n.Metadata.NodeName == nil {
			continue
		}
		if strings.TrimSpace(n.Metadata.Name) != targetName {
			continue
		}
		if n.Status != nil && n.Status.StatusCode == db.NETWORK_DELETING {
			continue
		}

		node := strings.TrimSpace(*n.Metadata.NodeName)
		if node == "" {
			continue
		}
		participantNodes[node] = struct{}{}

		if hubCandidate == "" && vxlanNetworkRole(n) == "head" {
			hubCandidate = node
		}
	}

	peerPolicy := vxlanPeerPolicy(vnet)
	if peerPolicy == api.Manual {
		return nil, nil
	}

	if len(participantNodes) == 0 {
		slog.Warn("vxlan participant discovery returned empty", "networkName", targetName, "networkId", api.VirtualNetworkID(vnet), "controllerNode", selfNode)
		return nil, nil
	}

	hubNode := strings.TrimSpace(hubCandidate)
	if hubNode == "" {
		hubNode = pickDeterministicHubNode(participantNodes)
		slog.Warn("vxlan hub node is not explicit; selected deterministic hub", "networkName", targetName, "networkId", api.VirtualNetworkID(vnet), "hubNode", hubNode)
	}

	peers := buildVxlanHubSpokePeerIPs(selfNode, hubNode, participantNodes, ipByNode)
	sort.Strings(peers)
	return peers, nil
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

func (c *controller) reconcileVxlanHubFailovers(vnets []api.VirtualNetwork, statuses []api.HostStatus) error {
	type networkGroup struct {
		name     string
		networks []api.VirtualNetwork
	}

	groups := map[string]*networkGroup{}
	for _, vnet := range vnets {
		if !isVxlanOverlay(vnet) {
			continue
		}
		peerPolicy := api.Auto
		if vnet.Spec.PeerPolicy != nil {
			peerPolicy = *vnet.Spec.PeerPolicy
		}
		if peerPolicy != api.Auto {
			continue
		}
		if vnet.Status != nil && vnet.Status.StatusCode == db.NETWORK_DELETING {
			continue
		}
		name := strings.TrimSpace(vnet.Metadata.Name)
		if name == "" {
			continue
		}
		group, exists := groups[name]
		if !exists {
			group = &networkGroup{name: name}
			groups[name] = group
		}
		group.networks = append(group.networks, vnet)
	}

	activeByNode := collectActiveHostStatusByNode(statuses, time.Now())
	for _, group := range groups {
		if err := c.reconcileVxlanHubFailoverForNetworkName(group.name, activeByNode); err != nil {
			return err
		}
	}

	return nil
}

func (c *controller) reconcileVxlanHubFailoverForNetworkName(networkName string, activeByNode map[string]api.HostStatus) error {
	name := strings.TrimSpace(networkName)
	if name == "" {
		return nil
	}

	mutex, err := c.db.LockKey(failoverLockKeyForNetworkName(name))
	if err != nil {
		return err
	}
	defer c.db.UnlockKey(mutex)

	networks, err := c.marmot.Db.GetVirtualNetworks()
	if err != nil {
		return err
	}

	group := make([]api.VirtualNetwork, 0, len(networks))
	for _, vnet := range networks {
		if !isVxlanOverlay(vnet) {
			continue
		}
		if vxlanPeerPolicy(vnet) != api.Auto {
			continue
		}
		if vnet.Status != nil && vnet.Status.StatusCode == db.NETWORK_DELETING {
			continue
		}
		if strings.TrimSpace(vnet.Metadata.Name) != name {
			continue
		}
		group = append(group, vnet)
	}

	return c.reconcileVxlanHubFailoverForGroup(group, activeByNode)
}

func (c *controller) reconcileVxlanHubFailoverForGroup(group []api.VirtualNetwork, activeByNode map[string]api.HostStatus) error {
	if len(group) == 0 {
		return nil
	}
	currentHub := resolveCurrentHubNode(group)
	if currentHub == "" {
		return nil
	}
	if _, ok := activeByNode[currentHub]; ok {
		return nil
	}

	participants := map[string]struct{}{}
	for _, vnet := range group {
		if vnet.Metadata.NodeName == nil {
			continue
		}
		node := strings.TrimSpace(*vnet.Metadata.NodeName)
		if node == "" {
			continue
		}
		participants[node] = struct{}{}
	}

	newHub := selectFailoverHubNode(participants, activeByNode)
	if newHub == "" {
		slog.Warn("vxlan hub failover skipped: no active participant available", "networkName", group[0].Metadata.Name, "oldHub", currentHub)
		return nil
	}
	if newHub == currentHub {
		return nil
	}

	promoted := findNetworkByNode(group, newHub)
	if promoted == nil {
		slog.Warn("vxlan hub failover skipped: selected hub has no network record", "networkName", group[0].Metadata.Name, "oldHub", currentHub, "newHub", newHub)
		return nil
	}
	oldHead := findHeadNetworkRecord(group)

	for _, network := range group {
		labels := ensureNetworkLabelsMap(network.Metadata.Labels)
		role := "follower"
		headNetworkID := api.VirtualNetworkID(*promoted)
		if api.VirtualNetworkID(network) == api.VirtualNetworkID(*promoted) {
			role = "head"
			headNetworkID = ""
			if network.Spec.IpNetworkId == nil && oldHead != nil && oldHead.Spec.IpNetworkId != nil {
				network.Spec.IpNetworkId = oldHead.Spec.IpNetworkId
			}
		} else {
			network.Spec.IpNetworkId = nil
		}
		db.SetNetworkSyncLabels(labels, role, headNetworkID, newHub)
		network.Metadata.Labels = &labels

		if err := c.db.UpdateVirtualNetworkById(api.VirtualNetworkID(network), network); err != nil {
			return err
		}
	}

	slog.Warn("vxlan hub failover completed", "networkName", group[0].Metadata.Name, "oldHub", currentHub, "newHub", newHub, "headNetworkId", api.VirtualNetworkID(*promoted))
	return nil
}

func ensureNetworkLabelsMap(src *map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(*src))
	for key, value := range *src {
		out[key] = value
	}
	return out
}

func findNetworkByNode(group []api.VirtualNetwork, nodeName string) *api.VirtualNetwork {
	target := strings.TrimSpace(nodeName)
	if target == "" {
		return nil
	}
	for i := range group {
		if group[i].Metadata.NodeName == nil {
			continue
		}
		if strings.TrimSpace(*group[i].Metadata.NodeName) == target {
			return &group[i]
		}
	}
	return nil
}

func findHeadNetworkRecord(group []api.VirtualNetwork) *api.VirtualNetwork {
	for i := range group {
		if vxlanNetworkRole(group[i]) == "head" {
			return &group[i]
		}
	}
	return nil
}

func resolveCurrentHubNode(group []api.VirtualNetwork) string {
	for _, vnet := range group {
		hub := vxlanHubNodeFromNetwork(vnet)
		if hub != "" {
			return hub
		}
	}

	participants := map[string]struct{}{}
	for _, vnet := range group {
		if vnet.Metadata.NodeName == nil {
			continue
		}
		node := strings.TrimSpace(*vnet.Metadata.NodeName)
		if node != "" {
			participants[node] = struct{}{}
		}
	}
	return pickDeterministicHubNode(participants)
}

func collectActiveHostStatusByNode(statuses []api.HostStatus, now time.Time) map[string]api.HostStatus {
	active := map[string]api.HostStatus{}
	cutoff := now.Add(-marmotd.ActiveHostThreshold)
	for _, st := range statuses {
		if st.NodeName == nil || st.LastUpdated == nil {
			continue
		}
		node := strings.TrimSpace(*st.NodeName)
		if node == "" {
			continue
		}
		if st.LastUpdated.After(cutoff) {
			active[node] = st
		}
	}
	return active
}

func selectFailoverHubNode(participants map[string]struct{}, activeByNode map[string]api.HostStatus) string {
	type candidate struct {
		nodeName      string
		hostID        uint32
		hasValidHostID bool
	}

	candidates := make([]candidate, 0, len(participants))
	for node := range participants {
		node = strings.TrimSpace(node)
		if node == "" {
			continue
		}
		status, ok := activeByNode[node]
		if !ok {
			continue
		}

		c := candidate{nodeName: node}
		if status.HostId != nil {
			if parsed, ok := parseHexHostID(*status.HostId); ok {
				c.hostID = parsed
				c.hasValidHostID = true
			}
		}
		candidates = append(candidates, c)
	}

	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hasValidHostID != candidates[j].hasValidHostID {
			return candidates[i].hasValidHostID
		}
		if candidates[i].hasValidHostID && candidates[i].hostID != candidates[j].hostID {
			return candidates[i].hostID < candidates[j].hostID
		}
		return candidates[i].nodeName < candidates[j].nodeName
	})

	return candidates[0].nodeName
}

func parseHexHostID(raw string) (uint32, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, false
	}
	v = strings.TrimPrefix(strings.ToLower(v), "0x")
	parsed, err := strconv.ParseUint(v, 16, 32)
	if err != nil || parsed == 0 {
		return 0, false
	}
	return uint32(parsed), true
}

func vxlanPeerPolicy(vnet api.VirtualNetwork) api.VirtualNetworkSpecPeerPolicy {
	if vnet.Spec.PeerPolicy == nil {
		return api.Auto
	}
	return *vnet.Spec.PeerPolicy
}

func failoverLockKeyForNetworkName(name string) string {
	sanitized := strings.TrimSpace(name)
	if sanitized == "" {
		sanitized = "unknown"
	}
	replacer := strings.NewReplacer("/", "_", " ", "_", "\t", "_")
	return "/lock/virtualnetwork/failover/" + replacer.Replace(sanitized)
}

func vxlanHubNodeFromNetwork(vnet api.VirtualNetwork) string {
	if vnet.Metadata.Labels != nil {
		headNode := strings.TrimSpace(db.GetHeadNetworkNodeName(*vnet.Metadata.Labels))
		if headNode != "" {
			return headNode
		}
	}
	if vnet.Metadata.NodeName != nil {
		return strings.TrimSpace(*vnet.Metadata.NodeName)
	}
	return ""
}

func vxlanNetworkRole(vnet api.VirtualNetwork) string {
	if vnet.Metadata.Labels == nil {
		return "head"
	}
	role := strings.TrimSpace(db.GetNetworkSyncRole(*vnet.Metadata.Labels))
	if role == "" {
		return "head"
	}
	return role
}

func pickDeterministicHubNode(nodes map[string]struct{}) string {
	if len(nodes) == 0 {
		return ""
	}
	candidates := make([]string, 0, len(nodes))
	for node := range nodes {
		candidates = append(candidates, node)
	}
	sort.Strings(candidates)
	return candidates[0]
}

func buildVxlanHubSpokePeerIPs(selfNode, hubNode string, participantNodes map[string]struct{}, ipByNode map[string]string) []string {
	self := strings.TrimSpace(selfNode)
	hub := strings.TrimSpace(hubNode)
	peers := make([]string, 0, len(participantNodes))
	peerSet := map[string]struct{}{}

	appendPeerIP := func(node string) {
		ip, ok := ipByNode[node]
		if !ok {
			slog.Warn("host status ip is missing for vxlan peer node", "node", node)
			return
		}
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if _, exists := peerSet[ip]; exists {
			return
		}
		peerSet[ip] = struct{}{}
		peers = append(peers, ip)
	}

	if self != "" && self != hub && hub != "" {
		appendPeerIP(hub)
		return peers
	}

	for node := range participantNodes {
		node = strings.TrimSpace(node)
		if node == "" || node == self {
			continue
		}
		appendPeerIP(node)
	}

	return peers
}
