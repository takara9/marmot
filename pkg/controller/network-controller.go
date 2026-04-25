package controller

import (
	"log/slog"
	"strings"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
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

		// 削除タイムスタンプが設定されて一定時間経過した仮想ネットワークのステータスをDELETINGに更新する
		// エラー中の仮想ネットワークは、対象にしない。
		if vnet.Status != nil && vnet.Status.StatusCode != db.NETWORK_ERROR {
			if vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
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
				slog.Debug("待ち状態の仮想ネットワークを処理", "networkId", vnet.Id)
				c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_PROVISIONING)
				if err := c.marmot.DeployVirtualNetwork(vnet); err != nil {
					slog.Error("DeployVirtualNetwork()", "err", err)
					c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					continue
				}
				c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ACTIVE)

			case db.NETWORK_PROVISIONING:
				slog.Debug("プロビジョニング中の仮想ネットワークを処理", "networkId", vnet.Id)

			case db.NETWORK_DELETING:
				slog.Debug("削除中の仮想ネットワークを処理", "networkId", vnet.Id)
				// 削除の処理で、libvirtで実態を消すのと、DBの状態も削除する。
				if err := c.marmot.DeleteVirtualNetwork(vnet.Id); err != nil {
					if strings.HasPrefix(err.Error(), "Network not found") {
						c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
						continue
					}
					slog.Error("DeleteVirtualNetwork()", "err", err)
					c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
					continue
				} else {
					slog.Debug("仮想ネットワークの削除成功", "networkId", vnet.Id)
				}
			case db.NETWORK_ERROR:
				slog.Debug("エラー状態の仮想ネットワークを処理", "networkId", vnet.Id)
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

			default:
				slog.Warn("不明なステータスの仮想ネットワークをスキップ", "networkId", vnet.Id, "status", *vnet.Status.Status)
			}
		}
	}
	// ワークキューから処理を取り出して、処理を実行する
}
