package controller

import (
	"log/slog"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	KUBERNETES_ENGINE_CONTROLLER_INTERVAL = 10 * time.Second
)

// kubernetesEngineController は MKS (Marmot Kubernetes Engine) コントローラーの雛形です。
// mke.json の読み込みと定期ループの骨組みのみを提供し、
// KubernetesEngine リソースのetcd監視・クラスタのライフサイクル管理は今後実装します。
type kubernetesEngineController struct {
	node     string
	mkeConf  *marmotd.MKEConfig
	stopChan chan struct{}
	doneChan chan struct{}
	stopOnce sync.Once
}

// KubernetesEngineコントローラーの開始
// mkeConfigPath に空文字を渡した場合は既定パス(marmotd.DefaultMKEConfigPath)を使用します。
func StartKubernetesEngineController(node string, etcdUrl string, mkeConfigPath string) (*kubernetesEngineController, error) {
	var c kubernetesEngineController
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

// KubernetesEngineコントローラーの制御ループ（雛形）
// TODO: /marmot 配下のKubernetesEngineオブジェクト監視、クラスタのライフサイクル管理を実装する
func (c *kubernetesEngineController) kubernetesEngineControllerLoop() {
	slog.Debug("KubernetesEngineコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))
}
