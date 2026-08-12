package controller

import (
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
	KUBERNETES_ENGINE_CONTROLLER_INTERVAL = 10 * time.Second
	KUBERNETES_ENGINE_DELETION_DELAY      = 15 * time.Second

	// kubernetesEngineNetworkNamePrefix はクラスタ専用のノード間通信ネットワーク名の接頭辞。
	// クラスタ名を含めることで他のネットワークと判別しやすくする。
	kubernetesEngineNetworkNamePrefix = "mke-"
)

var (
	provisionKubernetesEngineControlPlane = ProvisionKubernetesEngineControlPlane
	provisionKubernetesEngineNodes        = ProvisionKubernetesEngineNodes
)

// kubernetesEngineController は MKE (Marmot Kubernetes Engine) コントローラーです。
// mke.json の読み込みに加えて、/marmot/kubernetes-engine 配下を定期的にポーリングし、
// KubernetesEngine リソースのライフサイクル状態（Pending→Provisioning→Running→Deleting）を
// etcd に書き込みます。実際のクラスタ構築（VM作成・kubeadm等）は今後実装します。
type kubernetesEngineController struct {
	node     string
	etcdURL  string
	mkeConf  *marmotd.MKEConfig
	marmot   *marmotd.Marmot
	db       *db.Database
	stopChan chan struct{}
	doneChan chan struct{}
	stopOnce sync.Once
}

// KubernetesEngineコントローラーの開始
// mkeConfigPath に空文字を渡した場合は既定パス(marmotd.DefaultMKEConfigPath)を使用します。
func StartKubernetesEngineController(node string, etcdUrl string, mkeConfigPath string) (*kubernetesEngineController, error) {
	var c kubernetesEngineController
	var err error
	c.node = node
	c.etcdURL = etcdUrl

	if mkeConfigPath == "" {
		mkeConfigPath = marmotd.DefaultMKEConfigPath
	}

	cfg, err := marmotd.LoadMKEConfig(mkeConfigPath)
	if err != nil {
		slog.Error("Failed to load MKE config", "path", mkeConfigPath, "err", err)
		return nil, err
	}
	c.mkeConf = cfg
	slog.Debug("MKE設定を読み込みました", "path", mkeConfigPath, "kubernetesVersion", cfg.KubernetesVersion)

	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance for kubernetes engine controller", "err", err)
		return nil, err
	}
	c.db = c.marmot.Db

	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})

	// 定期実行の開始（10秒間隔）
	ticker := time.NewTicker(KUBERNETES_ENGINE_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.kubernetesEngineControllerLoop()
			case <-c.stopChan:
				slog.Debug("KubernetesEngineコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// KubernetesEngineコントローラーの停止
func (c *kubernetesEngineController) Stop() {
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

// kubernetesEngineControllerLoop は /marmot/kubernetes-engine 配下を毎tickポーリングし、
// 新規作成イベントの検知と状態遷移の駆動を行う（etcd watch APIは未使用、既存コントローラーと
// 同様のポーリング方式）。
func (c *kubernetesEngineController) kubernetesEngineControllerLoop() {
	slog.Debug("KubernetesEngineコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	items, err := c.db.GetKubernetesEngines()
	if err != nil {
		slog.Error("GetKubernetesEngines() failed", "err", err)
		return
	}

	for _, item := range items {
		id := api.KubernetesEngineID(item)

		if item.Status == nil {
			// 生成直後で状態未設定のレコードを検知した場合の雛形（通常は Create 時に PENDING が設定される）。
			slog.Debug("KubernetesEngineの生成を検知しました", "id", id, "name", item.Metadata.Name)
			_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PENDING, "")
			continue
		}

		if item.Status.DeletionTimeStamp != nil {
			if isKubernetesEngineInGracePeriod(item) {
				continue
			}
			if item.Status.StatusCode != db.KUBERNETES_ENGINE_DELETING {
				_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_DELETING, "")
				continue
			}
		}

		switch item.Status.StatusCode {
		case db.KUBERNETES_ENGINE_PENDING:
			c.reconcileKubernetesEnginePending(item)
		case db.KUBERNETES_ENGINE_PROVISIONING:
			c.reconcileKubernetesEngineProvisioning(item)
		case db.KUBERNETES_ENGINE_RUNNING:
			c.reconcileKubernetesEngineRunning(item)
		case db.KUBERNETES_ENGINE_DELETING:
			c.reconcileKubernetesEngineDeleting(item)
		case db.KUBERNETES_ENGINE_FAILED:
			slog.Debug("FAILED 状態のKubernetesEngineを検出", "id", id)
		default:
			slog.Warn("不明なKubernetesEngine状態", "id", id, "statusCode", item.Status.StatusCode)
		}
	}
}

// reconcileKubernetesEnginePending はクラスタ専用ネットワークを作成した上で PROVISIONING に進める。
// TODO: 次フェーズでノード用サーバーの作成を開始する。
func (c *kubernetesEngineController) reconcileKubernetesEnginePending(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)

	// ネットワーク作成とエンジン状態遷移をシリアライズするため、エンジンごとのロックを取得する。
	// 同一エンジンに対する DELETING パス（ネットワーク不在確認→ハードデリート）との競合を防ぐ。
	lockKey := "/lock/kubernetes-engine/reconcile/" + id
	mutex, err := c.db.LockKey(lockKey)
	if err != nil {
		slog.Warn("reconcileKubernetesEnginePending: failed to acquire lock", "id", id, "err", err)
		return
	}
	defer c.db.UnlockKey(mutex)

	// ロック取得後にエンジンの最新状態を再読み込みして、まだ PENDING であることを確認する。
	// 別インスタンスが先に処理を完了・削除していた場合はスキップする。
	current, err := c.db.GetKubernetesEngineById(id)
	if err != nil {
		slog.Warn("reconcileKubernetesEnginePending: GetKubernetesEngineById() failed", "id", id, "err", err)
		return
	}
	if current.Status == nil || current.Status.StatusCode != db.KUBERNETES_ENGINE_PENDING {
		return
	}

	if err := c.ensureKubernetesEngineNetwork(current); err != nil {
		slog.Warn("ensureKubernetesEngineNetwork() failed", "id", id, "err", err)
		return
	}
	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, "cluster provisioning started"); err != nil {
		slog.Warn("UpdateKubernetesEngineStatusWithMessage() failed", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineProvisioning はコントロールプレーンとノードを冪等に構成し、
// 全ノードがKubernetes API上でReadyになった時点でRUNNINGに進める。
func (c *kubernetesEngineController) reconcileKubernetesEngineProvisioning(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)
	lockKey := "/lock/kubernetes-engine/reconcile/" + id
	mutex, err := c.db.LockKey(lockKey)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineProvisioning: failed to acquire lock", "id", id, "err", err)
		return
	}
	defer c.db.UnlockKey(mutex)

	current, err := c.db.GetKubernetesEngineById(id)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineProvisioning: GetKubernetesEngineById() failed", "id", id, "err", err)
		return
	}
	if current.Status == nil || current.Status.StatusCode != db.KUBERNETES_ENGINE_PROVISIONING {
		return
	}
	if err := provisionKubernetesEngineControlPlane(c.db, c.mkeConf, c.etcdURL, current); err != nil {
		message := fmt.Sprintf("control plane provisioning failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, message)
		return
	}

	current, err = c.db.GetKubernetesEngineById(id)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineProvisioning: failed to reload control plane status", "id", id, "err", err)
		return
	}
	ready, err := provisionKubernetesEngineNodes(c.db, c.mkeConf, current)
	if err != nil {
		message := fmt.Sprintf("node provisioning failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, message)
		return
	}
	if !ready {
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, "waiting for Kubernetes nodes to become Ready")
		return
	}
	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		slog.Warn("UpdateKubernetesEngineStatusWithMessage() failed", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineRunning はRUNNING状態のヘルスチェック用の差し込み口。
// TODO: 次フェーズでノードの状態監視を行い、異常時はFAILED等へ遷移させる。
func (c *kubernetesEngineController) reconcileKubernetesEngineRunning(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)
	slog.Debug("KubernetesEngineはRUNNING状態です（ヘルスチェックは未実装）", "id", id, "name", ke.Metadata.Name)
}

// reconcileKubernetesEngineDeleting は猶予期間経過後に呼び出され、専用ネットワークの削除要求を行い、
// ネットワークの削除完了を確認してからetcdから実削除する。
// TODO: 次フェーズでノード(Server)等の関連リソース削除を先行させる。
func (c *kubernetesEngineController) reconcileKubernetesEngineDeleting(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)

	// ネットワーク不在確認とハードデリートをシリアライズするため、エンジンごとのロックを取得する。
	// 同一エンジンに対する PENDING パス（ネットワーク作成→状態遷移）との競合を防ぐ。
	lockKey := "/lock/kubernetes-engine/reconcile/" + id
	mutex, err := c.db.LockKey(lockKey)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineDeleting: failed to acquire lock", "id", id, "err", err)
		return
	}
	defer c.db.UnlockKey(mutex)

	// ロック取得後にエンジンの最新状態を再読み込みして、まだ DELETING であることを確認する。
	// 別インスタンスが先に削除を完了していた場合はスキップする。
	current, err := c.db.GetKubernetesEngineById(id)
	if err != nil {
		if err == db.ErrNotFound {
			return
		}
		slog.Warn("reconcileKubernetesEngineDeleting: GetKubernetesEngineById() failed", "id", id, "err", err)
		return
	}
	if current.Status == nil || current.Status.StatusCode != db.KUBERNETES_ENGINE_DELETING {
		return
	}

	networks, err := c.findKubernetesEngineNetworks(current)
	if err != nil {
		slog.Warn("findKubernetesEngineNetworks() failed", "id", id, "err", err)
		return
	}
	if len(networks) > 0 {
		// ヘッド/フォロワーを問わず、このクラスタが所有する全てのネットワークエントリに
		// 削除要求を出し、それら全てが消えるまでクラスタ本体の削除を待つ。
		for _, network := range networks {
			networkID := api.VirtualNetworkID(network)
			if network.Status == nil {
				// CreateVirtualNetwork()は必ずStatusを設定するため通常は到達しないが、
				// nilのままSetDeleteTimestampVirtualNetwork()を呼ぶとDB層でパニックするため回避する。
				slog.Warn("network status is nil, skip delete timestamp update", "id", id, "networkId", networkID)
				continue
			}
			if network.Status.DeletionTimeStamp == nil {
				if delErr := c.db.SetDeleteTimestampVirtualNetwork(networkID); delErr != nil {
					slog.Warn("SetDeleteTimestampVirtualNetwork() failed", "id", id, "networkId", networkID, "err", delErr)
				}
			}
		}
		// ネットワークの削除完了を待ってからクラスタ本体を削除する。
		return
	}

	if err := c.db.DeleteKubernetesEngineById(id); err != nil {
		slog.Warn("DeleteKubernetesEngineById() failed", "id", id, "err", err)
		return
	}
	slog.Debug("kubernetes engine deleted", "id", id)
}

// kubernetesEngineNetworkName はクラスタ専用のノード間通信ネットワーク名を返す。
// クラスタ名が未設定の場合は空文字を返す。
func kubernetesEngineNetworkName(ke api.KubernetesEngine) string {
	name := strings.TrimSpace(ke.Metadata.Name)
	if name == "" {
		return ""
	}
	return kubernetesEngineNetworkNamePrefix + name
}

// isKubernetesEngineOwnedNetwork はネットワークの所有者ラベルが指定のKubernetesEngine IDと一致し、
// かつmanagedByラベルがこのコントローラーの値であるかを判定する。ラベルは呼び出し元が自由に設定できる
// ため、両方一致した場合のみ「このコントローラーが管理するネットワーク」とみなす。
func isKubernetesEngineOwnedNetwork(network api.VirtualNetwork, kubernetesEngineID string) bool {
	if network.Metadata.Labels == nil {
		return false
	}
	labels := *network.Metadata.Labels
	owner, ok := labels[db.KubernetesEngineNetworkLabelOwner].(string)
	if !ok || owner != kubernetesEngineID {
		return false
	}
	managedBy, ok := labels[db.KubernetesEngineNetworkLabelManagedBy].(string)
	return ok && managedBy == db.KubernetesEngineNetworkLabelManagedByValue
}

// findKubernetesEngineNetworks はこのクラスタが所有する専用ネットワークのエントリを全て返す
// (ヘッドエントリおよびフォロワーエントリを含む)。所有者ラベル(KubernetesEngine ID)が一致する
// もののみを対象とする。1件も見つからない場合は空スライスを返す(エラーではない)。
func (c *kubernetesEngineController) findKubernetesEngineNetworks(ke api.KubernetesEngine) ([]api.VirtualNetwork, error) {
	id := api.KubernetesEngineID(ke)
	networks, err := c.db.GetVirtualNetworks()
	if err != nil {
		return nil, err
	}
	var owned []api.VirtualNetwork
	for _, n := range networks {
		if isKubernetesEngineOwnedNetwork(n, id) {
			owned = append(owned, n)
		}
	}
	return owned, nil
}

// ensureKubernetesEngineNetwork はクラスタ専用のノード間通信ネットワークを作成する。
// 既にこのクラスタが所有する同名ネットワークが存在すれば何もしない。名前が別クラスタ(または
// 無関係)のネットワークに使われている場合や、このクラスタが既に別名でネットワークを保有している
// 場合はエラーを返し、作成を行わない（ID・名前の両方で重複を検出する）。
func (c *kubernetesEngineController) ensureKubernetesEngineNetwork(ke api.KubernetesEngine) error {
	networkName := kubernetesEngineNetworkName(ke)
	if networkName == "" {
		return fmt.Errorf("kubernetes engine metadata.name is empty")
	}
	id := api.KubernetesEngineID(ke)

	// 名前の重複チェック: 同名のネットワークが既に存在する場合、所有者がこのクラスタかどうかを確認する。
	if existing, err := c.db.GetVirtualNetworkByName(networkName); err == nil {
		if isKubernetesEngineOwnedNetwork(existing, id) {
			return nil
		}
		return fmt.Errorf("network name %q already used by another network (id=%s)", networkName, api.VirtualNetworkID(existing))
	} else if err != db.ErrNotFound {
		return err
	}

	// IDの重複チェック: このクラスタが既に別名義でネットワークを保有していないか確認する。
	networks, err := c.db.GetVirtualNetworks()
	if err != nil {
		return err
	}
	for _, n := range networks {
		if isKubernetesEngineOwnedNetwork(n, id) {
			return fmt.Errorf("kubernetes engine %s already owns network %s", id, api.VirtualNetworkID(n))
		}
	}

	labels := map[string]interface{}{
		db.KubernetesEngineNetworkLabelOwner:     id,
		db.KubernetesEngineNetworkLabelManagedBy: db.KubernetesEngineNetworkLabelManagedByValue,
	}
	// ネットワークAPI(ApiCreateNetwork)と同様に、nodeNameとヘッドノード同期ラベルを設定する。
	// これらが無いとevaluateNodeAssignment()に弾かれ、ブリッジ/OVNが作成されないまま放置される。
	db.SetNetworkSyncLabels(labels, "head", "", c.node)
	network := api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: networkName, Labels: &labels},
	}
	if strings.TrimSpace(c.node) != "" {
		network.Metadata.NodeName = util.StringPtr(c.node)
	}
	if err := marmotd.ApplyVirtualNetworkDefaults(&network, marmotd.CurrentConfig(), c.db); err != nil {
		return err
	}
	if _, err := c.db.CreateVirtualNetwork(network); err != nil {
		return err
	}
	slog.Debug("KubernetesEngine用ネットワークを作成しました", "id", api.KubernetesEngineID(ke), "network", networkName)
	return nil
}

// isKubernetesEngineInGracePeriod は DeletionTimeStamp が設定されたレコードが
// まだ猶予期間内かどうかを返す。期間内は true を返し、ループはそのレコードをスキップする。
func isKubernetesEngineInGracePeriod(ke api.KubernetesEngine) bool {
	if ke.Status == nil || ke.Status.DeletionTimeStamp == nil {
		return false
	}
	return time.Since(*ke.Status.DeletionTimeStamp) <= KUBERNETES_ENGINE_DELETION_DELAY
}
