package controller

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
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
				c.networkControllerLoop()
			case <-c.stopChan:
				slog.Info("ネットワークコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) networkControllerLoop() {
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

	for _, vnet := range vnets {
		if ok, assignedNode, reason := evaluateNodeAssignment(vnet.Metadata, c.marmot.NodeName); !ok {
			objectName := ""
			if vnet.Metadata != nil && vnet.Metadata.Name != nil {
				objectName = *vnet.Metadata.Name
			}
			slog.Debug("別ノード割当の仮想ネットワークをスキップ", "networkId", vnet.Id, "networkName", objectName, "controllerNode", c.marmot.NodeName, "assignedNode", assignedNode, "reason", reason)
			continue
		}

		role := networkSyncRole(vnet.Metadata)

		// 削除タイムスタンプが設定されて一定時間経過した仮想ネットワークのステータスをDELETINGに更新する
		// エラー中の仮想ネットワークは、対象にしない。
		if vnet.Status != nil && vnet.Status.StatusCode != db.NETWORK_ERROR {
			if vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
				if err := c.distributeDeleteIntentToSameNameNetworks(vnet); err != nil {
					slog.Error("同名ネットワークへの削除意図配布に失敗", "networkId", vnet.Id, "err", err)
				}
				deletionTime := *vnet.Status.DeletionTimeStamp
				if time.Since(deletionTime) > c.deletionDelay {
					slog.Debug("削除のタイムスタンプが一定時間以上経過している仮想ネットワーク検出", "networkId", vnet.Id)
					c.marmot.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_DELETING)
					vnet.Status.StatusCode = db.NETWORK_DELETING
				}
			}
		}
		//fmt.Println("======================================================")
		//fmt.Println("仮想ネットワーク: ", "ID=", vnet.Id)
		//if vnet.Metadata != nil && vnet.Metadata.Name != nil {
		//	fmt.Println("ネットワーク 名前=", *vnet.Metadata.Name)
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
					c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_WAITING)
					continue
				}
				slog.Debug("待ち状態の仮想ネットワークを処理", "networkId", vnet.Id)
				if err := c.ensureFollowerNetworksWaiting(vnet); err != nil {
					slog.Error("フォロワー用ネットワークエントリーの作成に失敗", "headNetworkId", vnet.Id, "err", err)
				}
				c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_PROVISIONING)
				if err := c.marmot.DeployVirtualNetwork(vnet); err != nil {
					slog.Error("DeployVirtualNetwork()", "err", err)
					c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					continue
				}
				c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ACTIVE)

			case db.NETWORK_PROVISIONING:
				slog.Debug("プロビジョニング中の仮想ネットワークを処理", "networkId", vnet.Id)
				if role == "follower" {
					if err := c.reconcileFollowerWaitingNetwork(vnet); err != nil {
						slog.Error("フォロワーネットワークのプロビジョニング継続に失敗", "networkId", vnet.Id, "err", err)
						c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					}
				}

			case db.NETWORK_DELETING:
				slog.Debug("削除中の仮想ネットワークを処理", "networkId", vnet.Id)
				if role == "follower" {
					if err := c.ensureVirtualNetworkAbsent(vnet); err != nil {
						slog.Error("failed to delete virtual network on follower node", "err", err, "networkId", vnet.Id, "controllerNode", c.marmot.NodeName)
						c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
						continue
					}
					if err := c.db.DeleteVirtualNetworkById(vnet.Id); err != nil {
						slog.Error("failed to delete follower network object from DB", "err", err, "networkId", vnet.Id)
						c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					}
					continue
				}
				// 削除の処理で、libvirtで実態を消すのと、DBの状態も削除する。
				if err := c.marmot.DeleteVirtualNetwork(vnet.Id); err != nil {
					slog.Error("DeleteVirtualNetwork()", "err", err)
					c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					continue
				} else {
					slog.Debug("仮想ネットワークの削除成功", "networkId", vnet.Id)
				}
			case db.NETWORK_ERROR:
				slog.Debug("エラー状態の仮想ネットワークを処理", "networkId", vnet.Id)
				if role != "follower" {
					if err := c.distributeDeleteIntentToFollowerNetworks(vnet); err != nil {
						slog.Error("failed to distribute delete intent to follower networks", "headNetworkId", vnet.Id, "err", err)
					}
				}
				// エラーが発生していて、実態がなければ、DBから削除する　未実装
				_, found, err := c.marmot.Virt.GetVirtualNetworkByName(*vnet.Metadata.Name)
				if err != nil {
					slog.Error("Error checking if virtual network exists in libvirt", "err", err, "networkId", vnet.Id)
					if !found {
						slog.Debug("仮想ネットワークの実態が存在しないため、DBから削除", "networkId", vnet.Id)
						if err := c.db.DeleteVirtualNetworkById(vnet.Id); err != nil {
							slog.Error("Failed to delete virtual network from DB", "err", err, "networkId", vnet.Id)
							continue
						}
					} else {
						c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ACTIVE)
						slog.Debug("仮想ネットワークの実態が存在するため、DBのステータスをACTIVEに更新", "networkId", vnet.Id)
						continue
					}
				}
				// エラーが発生していて、実態も存在する場合は、次のループで削除の処理を再度試みる。

			case db.NETWORK_ACTIVE:
				slog.Debug("利用可能な仮想ネットワークを処理", "networkId", vnet.Id)
				if role == "follower" {
					if err := c.reconcileFollowerActiveNetwork(vnet); err != nil {
						slog.Error("failed to reconcile follower network", "err", err, "networkId", vnet.Id, "controllerNode", c.marmot.NodeName)
						c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					}
				}

			case db.NETWORK_WAITING:
				if role != "follower" {
					continue
				}
				slog.Debug("フォロワーネットワークはヘッドノード完了待ち", "networkId", vnet.Id)
				if err := c.reconcileFollowerWaitingNetwork(vnet); err != nil {
					slog.Error("フォロワーネットワークの同期開始に失敗", "networkId", vnet.Id, "err", err)
					c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
				}

			default:
				slog.Warn("不明なステータスの仮想ネットワークをスキップ", "networkId", vnet.Id, "status", *vnet.Status.Status)
			}
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
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
	if headNetwork.Metadata == nil || headNetwork.Metadata.Name == nil {
		return nil
	}

	headNode := ""
	if headNetwork.Metadata.NodeName != nil {
		headNode = strings.TrimSpace(*headNetwork.Metadata.NodeName)
	}
	if headNode == "" {
		return fmt.Errorf("head network nodeName is empty: networkId=%s", headNetwork.Id)
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
		newFollowerID, createErr := c.marmot.Db.MakeFollowerVirtualNetworkEntry(headNetwork, followerNode, headNetwork.Id)
		if createErr != nil {
			slog.Error("フォロワーネットワークエントリー作成失敗", "headNetworkId", headNetwork.Id, "followerNode", followerNode, "err", createErr)
			continue
		}
		slog.Debug("フォロワーネットワークをWAITINGで登録", "headNetworkId", headNetwork.Id, "followerNetworkId", newFollowerID, "followerNode", followerNode)
	}

	return nil
}

func (c *controller) distributeDeleteIntentToFollowerNetworks(headNetwork api.VirtualNetwork) error {
	if headNetwork.Metadata == nil || headNetwork.Metadata.Labels == nil {
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
		if network.Id == headNetwork.Id || network.Metadata == nil || network.Metadata.Labels == nil {
			continue
		}

		followerLabels := *network.Metadata.Labels
		if db.GetNetworkSyncRole(followerLabels) != "follower" {
			continue
		}
		if db.GetHeadNetworkID(followerLabels) != headNetwork.Id {
			continue
		}

		if network.Status != nil && network.Status.DeletionTimeStamp != nil {
			continue
		}

		if err := c.marmot.Db.SetDeleteTimestampVirtualNetwork(network.Id); err != nil {
			slog.Error("failed to set deletion timestamp to follower network", "headNetworkId", headNetwork.Id, "followerNetworkId", network.Id, "err", err)
			continue
		}

		slog.Debug("head network error detected: delete intent distributed to follower", "headNetworkId", headNetwork.Id, "followerNetworkId", network.Id)
	}

	return nil
}

func (c *controller) distributeDeleteIntentToSameNameNetworks(sourceNetwork api.VirtualNetwork) error {
	if sourceNetwork.Metadata == nil || sourceNetwork.Metadata.Name == nil {
		return nil
	}
	targetName := strings.TrimSpace(*sourceNetwork.Metadata.Name)
	if targetName == "" {
		return nil
	}

	networks, err := c.marmot.Db.GetVirtualNetworks()
	if err != nil {
		return err
	}

	for _, network := range networks {
		if network.Id == sourceNetwork.Id {
			continue
		}
		if network.Metadata == nil || network.Metadata.Name == nil {
			continue
		}
		if strings.TrimSpace(*network.Metadata.Name) != targetName {
			continue
		}
		if network.Status != nil && network.Status.DeletionTimeStamp != nil {
			continue
		}

		if err := c.marmot.Db.SetDeleteTimestampVirtualNetwork(network.Id); err != nil {
			slog.Error("同名ネットワークへの削除タイムスタンプ設定に失敗", "sourceNetworkId", sourceNetwork.Id, "targetNetworkId", network.Id, "networkName", targetName, "err", err)
			continue
		}

		slog.Debug("同名ネットワークへ削除意図を配布", "sourceNetworkId", sourceNetwork.Id, "targetNetworkId", network.Id, "networkName", targetName)
	}

	return nil
}

func (c *controller) reconcileFollowerWaitingNetwork(waitingNetwork api.VirtualNetwork) error {
	if waitingNetwork.Metadata == nil || waitingNetwork.Metadata.Labels == nil {
		return fmt.Errorf("labels are required for waiting network: networkId=%s", waitingNetwork.Id)
	}
	labels := *waitingNetwork.Metadata.Labels
	headNetworkID := db.GetHeadNetworkID(labels)
	if headNetworkID == "" {
		return fmt.Errorf("headNetworkId label is missing: networkId=%s", waitingNetwork.Id)
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
		c.db.UpdateVirtualNetworkStatus(waitingNetwork.Id, db.NETWORK_PROVISIONING)
		if err := c.ensureVirtualNetworkPresent(headNetwork); err != nil {
			return err
		}
		c.db.UpdateVirtualNetworkStatus(waitingNetwork.Id, db.NETWORK_ACTIVE)
	case db.NETWORK_DELETING:
		c.db.UpdateVirtualNetworkStatus(waitingNetwork.Id, db.NETWORK_DELETING)
	case db.NETWORK_ERROR:
		return fmt.Errorf("head network is not available: headNetworkId=%s status=%s", headNetworkID, db.NetworkStatus[headNetwork.Status.StatusCode])
	default:
		// WAITING を維持する。
	}

	return nil
}

func (c *controller) reconcileFollowerActiveNetwork(followerNetwork api.VirtualNetwork) error {
	if followerNetwork.Metadata == nil || followerNetwork.Metadata.Labels == nil {
		return fmt.Errorf("labels are required for follower network: networkId=%s", followerNetwork.Id)
	}
	labels := *followerNetwork.Metadata.Labels
	headNetworkID := db.GetHeadNetworkID(labels)
	if headNetworkID == "" {
		return fmt.Errorf("headNetworkId label is missing: networkId=%s", followerNetwork.Id)
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
		return c.ensureVirtualNetworkPresent(followerNetwork)
	case db.NETWORK_DELETING:
		c.db.UpdateVirtualNetworkStatus(followerNetwork.Id, db.NETWORK_DELETING)
		return nil
	default:
		return nil
	}
}

func (c *controller) ensureVirtualNetworkPresent(vnet api.VirtualNetwork) error {
	if vnet.Metadata == nil || vnet.Metadata.Name == nil || strings.TrimSpace(*vnet.Metadata.Name) == "" {
		return fmt.Errorf("network metadata.name is required: networkId=%s", vnet.Id)
	}

	if _, found, err := c.marmot.Virt.GetVirtualNetworkByName(*vnet.Metadata.Name); err == nil && found {
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
	if vnet.Metadata == nil || vnet.Metadata.Name == nil || strings.TrimSpace(*vnet.Metadata.Name) == "" {
		return fmt.Errorf("network metadata.name is required: networkId=%s", vnet.Id)
	}

	_, found, err := c.marmot.Virt.GetVirtualNetworkByName(*vnet.Metadata.Name)
	if err != nil || !found {
		// 既に存在しない場合は何もしない
		return nil
	}

	if err := c.marmot.Virt.DeleteVirtualNetwork(*vnet.Metadata.Name); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return err
	}

	slog.Debug("フォロワーノードで仮想ネットワーク実体を削除", "networkId", vnet.Id, "networkName", *vnet.Metadata.Name, "controllerNode", c.marmot.NodeName)
	return nil
}
