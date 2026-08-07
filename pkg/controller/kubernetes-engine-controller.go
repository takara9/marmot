package controller

import (
	"log/slog"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	KUBERNETES_ENGINE_CONTROLLER_INTERVAL = 10 * time.Second
	KUBERNETES_ENGINE_DELETION_DELAY      = 15 * time.Second
)

// kubernetesEngineController は MKE (Marmot Kubernetes Engine) コントローラーです。
// mke.json の読み込みに加えて、/marmot/kubernetes-engine 配下を定期的にポーリングし、
// KubernetesEngine リソースのライフサイクル状態（Pending→Provisioning→Running→Deleting）を
// etcd に書き込みます。実際のクラスタ構築（VM作成・kubeadm等）は今後実装します。
type kubernetesEngineController struct {
	node     string
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

	if mkeConfigPath == "" {
		mkeConfigPath = marmotd.DefaultMKEConfigPath
	}

	cfg, err := marmotd.LoadMKEConfig(mkeConfigPath)
	if err != nil {
		slog.Error("Failed to load MKE config", "path", mkeConfigPath, "err", err)
		return nil, err
	}
	c.mkeConf = cfg
	slog.Info("MKE設定を読み込みました", "path", mkeConfigPath, "kubernetesVersion", cfg.KubernetesVersion)

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

// reconcileKubernetesEnginePending は新規作成を検知したクラスタを PROVISIONING に進める。
// TODO: 次フェーズでノード用サーバーの作成を開始する。
func (c *kubernetesEngineController) reconcileKubernetesEnginePending(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)
	if err := c.db.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, "cluster provisioning started"); err != nil {
		slog.Warn("UpdateKubernetesEngineStatusWithMessage() failed", "id", id, "err", err)
	}
}

// reconcileKubernetesEngineProvisioning はノード起動状況を確認して RUNNING に進める処理の差し込み口。
// TODO: 次フェーズでノード(Server)の起動完了を確認し、全ノード Running になった時点で RUNNING に遷移させる。
func (c *kubernetesEngineController) reconcileKubernetesEngineProvisioning(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)
	slog.Debug("KubernetesEngineはPROVISIONING状態です（ノード起動確認は未実装）", "id", id, "name", ke.Metadata.Name)
}

// reconcileKubernetesEngineRunning はRUNNING状態のヘルスチェック用の差し込み口。
// TODO: 次フェーズでノードの状態監視を行い、異常時はFAILED等へ遷移させる。
func (c *kubernetesEngineController) reconcileKubernetesEngineRunning(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)
	slog.Debug("KubernetesEngineはRUNNING状態です（ヘルスチェックは未実装）", "id", id, "name", ke.Metadata.Name)
}

// reconcileKubernetesEngineDeleting は猶予期間経過後に呼び出され、etcdから実削除する。
// TODO: 次フェーズでノード(Server)等の関連リソース削除を先行させる。
func (c *kubernetesEngineController) reconcileKubernetesEngineDeleting(ke api.KubernetesEngine) {
	id := api.KubernetesEngineID(ke)
	if err := c.db.DeleteKubernetesEngineById(id); err != nil {
		slog.Warn("DeleteKubernetesEngineById() failed", "id", id, "err", err)
		return
	}
	slog.Debug("kubernetes engine deleted", "id", id)
}

// isKubernetesEngineInGracePeriod は DeletionTimeStamp が設定されたレコードが
// まだ猶予期間内かどうかを返す。期間内は true を返し、ループはそのレコードをスキップする。
func isKubernetesEngineInGracePeriod(ke api.KubernetesEngine) bool {
	if ke.Status == nil || ke.Status.DeletionTimeStamp == nil {
		return false
	}
	return time.Since(*ke.Status.DeletionTimeStamp) <= KUBERNETES_ENGINE_DELETION_DELAY
}
