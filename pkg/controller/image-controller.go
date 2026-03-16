package controller

import (
	"log/slog"
	"time"

	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	IMAGE_CONTROLLER_INTERVAL = 5 * time.Second
)

// VMコントローラーの開始
func StartImageController(node string, etcdUrl string) (*controller, error) {
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

	// 定期実行の開始
	ticker := time.NewTicker(IMAGE_CONTROLLER_INTERVAL)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.imageControllerLoop()
			}
		}
	}()
	return &c, nil
}

// コントローラーの制御ループ
func (c *controller) imageControllerLoop() {
	slog.Info("イメージコントローラーの制御ループ実行", "CONTROLLER", time.Now().Format(time.DateTime))

}
