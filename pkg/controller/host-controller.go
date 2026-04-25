package controller

import (
	"log/slog"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	HOST_CONTROLLER_INTERVAL = 10 * time.Second
)

type hostController struct {
	marmot   *marmotd.Marmot
	stopChan chan struct{}
	doneChan chan struct{}
	stopOnce sync.Once
}

// ホストコントローラーの開始
func StartHostController(node string, etcdUrl string) (*hostController, error) {
	var c hostController
	var err error

	c.stopChan = make(chan struct{})
	c.doneChan = make(chan struct{})
	c.marmot, err = marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance for host controller", "err", err)
		return nil, err
	}

	// 起動時に最初のデータ収集を実行
	if err := c.marmot.CollectAndUpdateHostStatus(); err != nil {
		slog.Error("Initial host status collection failed", "err", err)
		// 起動時エラーはログのみで続行する
	}

	// 定期実行の開始（10秒間隔）
	ticker := time.NewTicker(HOST_CONTROLLER_INTERVAL)
	go func() {
		defer ticker.Stop()
		defer close(c.doneChan)
		for {
			select {
			case <-ticker.C:
				c.hostControllerLoop()
			case <-c.stopChan:
				slog.Info("ホストコントローラー停止")
				return
			}
		}
	}()
	return &c, nil
}

// ホストコントローラーの停止
func (c *hostController) Stop() {
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

// ホストコントローラーの制御ループ
func (c *hostController) hostControllerLoop() {
	slog.Debug("ホストコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format("2006-01-02 15:04:05"))

	if err := c.marmot.CollectAndUpdateHostStatus(); err != nil {
		slog.Error("Failed to collect and update host status", "err", err)
	}
}
