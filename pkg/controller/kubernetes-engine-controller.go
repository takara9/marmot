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
	provisionKubernetesEngineControlPlane  = ProvisionKubernetesEngineControlPlane
	provisionKubernetesEngineNodes         = ProvisionKubernetesEngineNodes
	provisionKubernetesEngineLoadBalancer  = ProvisionKubernetesEngineLoadBalancer
	reconcileKubernetesEngineRunningRoutes = reconcileKubernetesEngineRunningNodeRoutes
	drainAndDeleteKubernetesEngineNode     = DrainAndDeleteKubernetesEngineNode
	upgradeKubernetesEngineControlPlane    = UpgradeKubernetesEngineControlPlane
	upgradeKubernetesEngineNode            = UpgradeKubernetesEngineNode

	// releaseKubernetesEngineLoadBalancerVipsFn / removeKubernetesEngineCephRBDImagesFn は
	// 削除フローの差し替え口(単体テストで実際のクラスタ・Ceph無しに検証するため)。
	releaseKubernetesEngineLoadBalancerVipsFn = releaseKubernetesEngineLoadBalancerVips
	removeKubernetesEngineCephRBDImagesFn     = removeKubernetesEngineCephRBDImages
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
		case db.KUBERNETES_ENGINE_RUNNING, db.KUBERNETES_ENGINE_SCALING_OUT, db.KUBERNETES_ENGINE_SCALING_IN, db.KUBERNETES_ENGINE_UPGRADING:
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
	// ロードバランサー仮想サーバーの status.creationTimeStamp がノードより早くなるよう、
	// ノードのReady待ちより先に呼び出す(互いに依存しないため並行して進めても問題ない)。
	loadBalancerReady, err := provisionKubernetesEngineLoadBalancer(c.db, c.mkeConf, current)
	if err != nil {
		message := fmt.Sprintf("load balancer provisioning failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, message)
		return
	}

	nodesReady, err := provisionKubernetesEngineNodes(c.db, c.mkeConf, current)
	if err != nil {
		message := fmt.Sprintf("node provisioning failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, message)
		return
	}

	if !loadBalancerReady {
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, "waiting for load balancer server to become ready")
		return
	}
	if !nodesReady {
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, "waiting for Kubernetes nodes to become Ready")
		return
	}

	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		slog.Warn("UpdateKubernetesEngineStatusWithMessage() failed", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineRunning はRUNNING状態のヘルスチェック用の差し込み口。
// marmotdホストの再起動等でコントロールプレーン専用ネットワークネームスペース
// (/run/netns配下はtmpfsのため再起動で消える)が失われると、専用etcd/kube-apiserver等の
// systemdユニットがNetworkNamespacePath不在で起動失敗し続けるため、ここで検知して復旧する。
// また、ノードVMの再起動で失われたPod CIDR宛の静的経路(`ip route replace`は永続化されない)も
// 毎tick再設定し、Service(NodePort等)の疎通が失われたままにならないようにする。
// spec.nodesが現在のノード数を上回っている場合、または既にSCALING_OUT中の場合は、
// スケールアウト処理へ委譲する(完了するまでRUNNINGへの他の処理は行わない)。
// spec.nodesが現在のノード数を下回っている場合、または既にSCALING_IN中の場合は、
// スケールイン処理へ委譲する。
// TODO: 次フェーズでノードの状態監視を行い、異常時はFAILED等へ遷移させる。
func (c *kubernetesEngineController) reconcileKubernetesEngineRunning(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)

	if ke.Status != nil && ke.Status.StatusCode == db.KUBERNETES_ENGINE_SCALING_OUT {
		c.reconcileKubernetesEngineScaleOut(id, ke)
		return
	}
	if ke.Status != nil && ke.Status.StatusCode == db.KUBERNETES_ENGINE_SCALING_IN {
		c.reconcileKubernetesEngineScaleIn(id, ke)
		return
	}
	if ke.Status != nil && ke.Status.StatusCode == db.KUBERNETES_ENGINE_UPGRADING {
		c.reconcileKubernetesEngineUpgrading(id, ke)
		return
	}

	servers, err := findKubernetesEngineNodeServers(c.db, ke)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineRunning: failed to list node servers", "id", id, "err", err)
	} else {
		active := 0
		for _, server := range servers {
			if server.Status == nil || server.Status.DeletionTimeStamp == nil {
				active++
			}
		}
		if active < ke.Spec.Nodes {
			if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_OUT, "adding nodes to match spec.nodes"); err != nil {
				slog.Warn("reconcileKubernetesEngineRunning: failed to transition to SCALING_OUT", "id", id, "err", err)
				return
			}
			c.reconcileKubernetesEngineScaleOut(id, ke)
			return
		}
		if active > ke.Spec.Nodes {
			if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_IN, "removing nodes to match spec.nodes"); err != nil {
				slog.Warn("reconcileKubernetesEngineRunning: failed to transition to SCALING_IN", "id", id, "err", err)
				return
			}
			c.reconcileKubernetesEngineScaleIn(id, ke)
			return
		}
		if active == ke.Spec.Nodes {
			needsUpgrade, err := kubernetesEngineNeedsUpgrade(ke)
			if err != nil {
				slog.Warn("reconcileKubernetesEngineRunning: failed to determine upgrade need", "id", id, "err", err)
			} else if needsUpgrade {
				if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, "upgrading Kubernetes version"); err != nil {
					slog.Warn("reconcileKubernetesEngineRunning: failed to transition to UPGRADING", "id", id, "err", err)
					return
				}
				c.reconcileKubernetesEngineUpgrading(id, ke)
				return
			}
		}
	}

	slog.Debug("KubernetesEngineはRUNNING状態です", "id", id, "name", ke.Metadata.Name)

	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(ke.Metadata.Name)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineRunning: failed to resolve control plane namespace name", "id", id, "err", err)
		return
	}
	if !controlPlaneNamespaceExists(namespace) {
		slog.Warn("コントロールプレーンのネットワークネームスペースが失われています。復旧を試みます", "id", id, "namespace", namespace)
		if err := provisionKubernetesEngineControlPlane(c.db, c.mkeConf, c.etcdURL, ke); err != nil {
			message := fmt.Sprintf("control plane network recovery failed: %v", err)
			_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, message)
			slog.Warn("reconcileKubernetesEngineRunning: control plane recovery failed", "id", id, "err", err)
			return
		}
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, "")
		slog.Debug("コントロールプレーンのネットワークネームスペースを復旧しました", "id", id, "namespace", namespace)
	}

	if err := reconcileKubernetesEngineRunningRoutes(c.db, ke); err != nil {
		message := fmt.Sprintf("pod network route reconciliation failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, message)
		slog.Warn("reconcileKubernetesEngineRunning: node route reconciliation failed", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineUpgrading はUPGRADING中に呼び出され、spec.versionに合わせて
// コントロールプレーン(kube-apiserver/kube-scheduler/kube-controller-manager、および専用etcdの
// 再確認)を先に更新し、完了後にノードをindex昇順で1台ずつcordon→drain[4bで実装したドレイン処理を
// 流用]→kubelet/kube-proxyバイナリの強制差し替え→uncordon→Ready確認まで進める。
// 1tickにつきコントロールプレーン更新またはノード1台分のみ進め、エラー時はメッセージを記録して
// 停止する(自動ロールバックは行わない)。全ノードが最新バージョンになった時点でRUNNINGへ戻す。
func (c *kubernetesEngineController) reconcileKubernetesEngineUpgrading(id string, ke api.KubernetesEngine) {
	needsControlPlaneUpgrade, err := kubernetesEngineNeedsUpgrade(ke)
	if err != nil {
		message := fmt.Sprintf("upgrade failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, message)
		slog.Warn("reconcileKubernetesEngineUpgrading: failed to determine control plane upgrade need", "id", id, "err", err)
		return
	}
	if needsControlPlaneUpgrade {
		if err := upgradeKubernetesEngineControlPlane(c.db, c.mkeConf, c.etcdURL, ke); err != nil {
			message := fmt.Sprintf("control plane upgrade failed: %v", err)
			_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, message)
			slog.Warn("reconcileKubernetesEngineUpgrading: control plane upgrade failed", "id", id, "err", err)
			return
		}
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, "control plane upgraded, upgrading nodes")
		return
	}

	target, err := selectKubernetesEngineNodeForUpgrade(c.db, ke)
	if err != nil {
		message := fmt.Sprintf("upgrade failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, message)
		slog.Warn("reconcileKubernetesEngineUpgrading: node selection failed", "id", id, "err", err)
		return
	}
	if target != nil {
		if err := upgradeKubernetesEngineNode(c.db, c.mkeConf, ke, *target); err != nil {
			message := fmt.Sprintf("node upgrade failed: %v", err)
			_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, message)
			slog.Warn("reconcileKubernetesEngineUpgrading: node upgrade failed", "id", id, "node", target.Metadata.Name, "err", err)
			return
		}
		return
	}

	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		slog.Warn("reconcileKubernetesEngineUpgrading: failed to transition back to RUNNING", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineScaleOut はSCALING_OUT中に呼び出され、不足ノードの作成・セットアップを
// 冪等に進める。全ノードがReadyになった時点でRUNNINGへ戻す。
func (c *kubernetesEngineController) reconcileKubernetesEngineScaleOut(id string, ke api.KubernetesEngine) {
	ready, err := provisionKubernetesEngineNodes(c.db, c.mkeConf, ke)
	if err != nil {
		message := fmt.Sprintf("scale-out failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_OUT, message)
		slog.Warn("reconcileKubernetesEngineScaleOut: node provisioning failed", "id", id, "err", err)
		return
	}
	if !ready {
		return
	}
	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		slog.Warn("reconcileKubernetesEngineScaleOut: failed to transition back to RUNNING", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineScaleIn はSCALING_IN中に呼び出され、spec.nodesを超過する
// アクティブノードのうちindex最大の1台を選んでcordon→drain→ノード削除→VM削除要求まで
// 1tickにつき1台ずつ進める(可用性維持のためのローリング処理)。超過が解消し、既存の
// 削除要求も完了していればRUNNINGへ戻す。
func (c *kubernetesEngineController) reconcileKubernetesEngineScaleIn(id string, ke api.KubernetesEngine) {
	target, err := selectKubernetesEngineNodeForScaleIn(c.db, ke)
	if err != nil {
		message := fmt.Sprintf("scale-in failed: %v", err)
		_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_IN, message)
		slog.Warn("reconcileKubernetesEngineScaleIn: node selection failed", "id", id, "err", err)
		return
	}
	if target != nil {
		if err := drainAndDeleteKubernetesEngineNode(ke, target.Metadata.Name); err != nil {
			message := fmt.Sprintf("scale-in failed: %v", err)
			_ = c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_IN, message)
			slog.Warn("reconcileKubernetesEngineScaleIn: drain/delete node failed", "id", id, "node", target.Metadata.Name, "err", err)
			return
		}
		serverID := api.ServerID(*target)
		if err := c.db.SetDeleteTimestamp(serverID); err != nil {
			slog.Warn("reconcileKubernetesEngineScaleIn: SetDeleteTimestamp failed", "id", id, "serverId", serverID, "err", err)
		}
		return
	}

	// アクティブノードは既にspec.nodes以下。削除要求済みのVMがまだ残っていれば完了を待つ。
	servers, err := findKubernetesEngineNodeServers(c.db, ke)
	if err != nil {
		slog.Warn("reconcileKubernetesEngineScaleIn: failed to list node servers", "id", id, "err", err)
		return
	}
	for _, server := range servers {
		if server.Status != nil && server.Status.DeletionTimeStamp != nil {
			return
		}
	}
	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		slog.Warn("reconcileKubernetesEngineScaleIn: failed to transition back to RUNNING", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineDeleting は猶予期間経過後に呼び出され、ノード用仮想サーバーの削除要求、
// コントロールプレーンの解体（systemdユニット・etcd・netns・専用IPの解放）と専用ネットワークの
// 削除要求を行い、それらの削除完了を確認してからetcdから実削除する。
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

	// フェーズ11で払い出したVIP(host-bridge IPAM)と内部DNSエントリーは、クラスタ全体削除時には
	// mke専用ロードバランサー(mke-lb-controller)がServiceの消失を検知して個別に解放する機会を
	// 失うため、ここで一括解放する。ベストエフォートとし、失敗してもクラスタ削除は継続する。
	if err := releaseKubernetesEngineLoadBalancerVipsFn(c.db, current); err != nil {
		slog.Warn("releaseKubernetesEngineLoadBalancerVips() failed", "id", id, "err", err)
	}

	// ネットワークやコントロールプレーンを解体する前に、このクラスタが所有するノード用仮想サーバーの
	// 削除を要求し、それら全てが消えるまで待つ。VMがネットワークに接続されたまま専用ネットワークが
	// 削除されるのを避けるため、サーバー削除を先行させる。mke専用ロードバランサー用仮想サーバー
	// (存在する場合)も同様にここで削除対象へ含める。
	nodeServers, err := findKubernetesEngineNodeServers(c.db, current)
	if err != nil {
		slog.Warn("findKubernetesEngineNodeServers() failed", "id", id, "err", err)
		return
	}
	loadBalancerServer, err := findKubernetesEngineLoadBalancerServer(c.db, current)
	if err != nil {
		slog.Warn("findKubernetesEngineLoadBalancerServer() failed", "id", id, "err", err)
		return
	}
	if loadBalancerServer != nil {
		if loadBalancerServer.Status == nil || loadBalancerServer.Status.DeletionTimeStamp == nil {
			revokeKubernetesEngineLoadBalancerApiKey(c.db, *loadBalancerServer)
		}
		nodeServers = append(nodeServers, *loadBalancerServer)
	}
	if len(nodeServers) > 0 {
		if current.Status != nil && current.Status.ControlPlaneIpAddress != nil {
			// ノード用VM(Ceph-CSIのprovisioner Podの実行元)がまだ生きている最後のタイミングで、
			// Ceph-CSI(RBD)が払い出し済みのPVCをkube-apiserver経由で削除し、RBD imageの実削除を
			// Ceph-CSI本来の削除経路(watcher解放等を含む)に委ねる(CephFSサブボリュームは対象外)。
			// ベストエフォートとし、失敗してもノード用サーバーの削除は継続する。
			if cephErr := removeKubernetesEngineCephRBDImagesFn(current); cephErr != nil {
				slog.Warn("removeKubernetesEngineCephRBDImages() failed", "id", id, "err", cephErr)
			}
		}
		for _, server := range nodeServers {
			if server.Status != nil && server.Status.DeletionTimeStamp != nil {
				continue
			}
			serverID := api.ServerID(server)
			if delErr := c.db.SetDeleteTimestamp(serverID); delErr != nil {
				slog.Warn("SetDeleteTimestamp() failed", "id", id, "serverId", serverID, "err", delErr)
			}
		}
		// ノード用サーバーの削除完了を待ってからネットワーク・コントロールプレーンの解体に進む。
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
		deprovisionAttempted := false
		for _, network := range networks {
			networkID := api.VirtualNetworkID(network)
			if network.Status == nil {
				// CreateVirtualNetwork()は必ずStatusを設定するため通常は到達しないが、
				// nilのままSetDeleteTimestampVirtualNetwork()を呼ぶとDB層でパニックするため回避する。
				slog.Warn("network status is nil, skip delete timestamp update", "id", id, "networkId", networkID)
				continue
			}
			// このクラスタ自身のノード間通信ネットワークについては、削除要求のタイムスタンプが
			// 既に設定済み（=以前のreconcileで削除待ちに入った後）でも、コントロールプレーン
			// （systemdユニット・etcd・netns・専用IP）が未解体ならここで解体する。
			// 解体しないとコントロールプレーンIPがIPAM上に残り続け、
			// CheckIPnetInUse()が常にtrueを返してネットワーク削除が永久にブロックされる。
			if !deprovisionAttempted &&
				network.Metadata.Name == kubernetesEngineNetworkName(current) &&
				current.Status != nil && current.Status.ControlPlaneIpAddress != nil {
				deprovisionAttempted = true
				if deprovErr := DeprovisionKubernetesEngineControlPlane(c.db, c.mkeConf, current); deprovErr != nil {
					slog.Warn("DeprovisionKubernetesEngineControlPlane() failed", "id", id, "err", deprovErr)
				}
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
