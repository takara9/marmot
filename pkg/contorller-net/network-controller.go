package controller_net

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	CONTROLLER_INTERVAL = 5 * time.Second
)

var controllerCounter uint64 = 0

type controller struct {
	db     *db.Database
	Lock   sync.Mutex
	marmot *marmotd.Marmot
}

// ネットワークコントローラーの開始
func StartNetController(node string, etcdUrl string) (*controller, error) {
	var c controller
	var err error

	// 初期化
	// marmotd との接続設定
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db // 正しくないけど

	// 起動時に既存の仮想ネットワークを取得して、データベースに登録する
	if _, err := c.marmot.GetVirtualNetworksAndPutDB(); err != nil {
		slog.Error("Failed to get virtual networks and put DB", "err", err)
		return nil, err
	}

	// 定期実行の開始
	ticker := time.NewTicker(CONTROLLER_INTERVAL)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.controllerLoop()
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) controllerLoop() {
	slog.Info("ネットワークコントローラーの制御ループ実行", "CONTROLLER", controllerCounter)
	controllerCounter++
	vnets, err := c.marmot.GetVirtualNetwork()
	if err != nil {
		slog.Error("failed to get virtual networks", "err", err)
		return
	}
	for _, vnet := range vnets {

		//byte, err := json.MarshalIndent(vnet, "", "  ")
		//if err != nil {
		//	slog.Error("failed to marshal virtual network", "err", err)
		//} else {
		//	fmt.Println("仮想ネットワークのJSON情報", "json", string(byte))
		//}

		// 削除タイムスタンプが設定されて一定時間経過した仮想ネットワークのステータスをDELETINGに更新する
		if vnet.Status != nil && vnet.Status.DeletionTimeStamp != nil {
			deletionTime := *vnet.Status.DeletionTimeStamp
			if time.Since(deletionTime) > 30*time.Second {
				slog.Debug("削除のタイムスタンプが一定時間以上経過している仮想ネットワーク検出", "networkId", vnet.Id)
				c.marmot.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_DELETING)
				vnet.Status.Status = util.IntPtrInt(db.NETWORK_DELETING)
			}
		}

		fmt.Println("仮想ネットワーク: ", "ID=", vnet.Id, "NAME=", *vnet.Metadata.Name, "STATUS=", db.NetworkStatus[*vnet.Status.Status])

		switch *vnet.Status.Status {
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
			if err := c.marmot.DeleteVirtualNetwork(vnet.Id); err != nil {
				slog.Error("DeleteVirtualNetwork()", "err", err)
				c.db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
				continue
			}
			slog.Debug("仮想ネットワークの削除成功", "networkId", vnet.Id)

		case db.NETWORK_ERROR:
			slog.Debug("エラー状態の仮想ネットワークを処理", "networkId", vnet.Id)

		case db.NETWORK_ACTIVE:
			slog.Debug("利用可能な仮想ネットワークを処理", "networkId", vnet.Id)

		default:
			slog.Warn("不明なステータスの仮想ネットワークをスキップ", "networkId", vnet.Id, "status", *vnet.Status.Status)
		}

	}
	// ワークキューから処理を取り出して、処理を実行する
}
