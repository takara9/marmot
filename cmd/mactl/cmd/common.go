package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/config"
)

// creationTime は Status から作成日時を返す。nil の場合はゼロ時刻を返す。
func creationTime(s *api.Status) time.Time {
	if s != nil && s.CreationTimeStamp != nil {
		return *s.CreationTimeStamp
	}
	return time.Time{}
}

// deletionMarker は削除要求済み(DeletionTimeStamp != nil)なら "*" を返す。
func deletionMarker(status *api.Status) string {
	if status != nil && status.DeletionTimeStamp != nil {
		return "*"
	}
	return ""
}

// コンフィグからエンドポイントを取り出してセットする
//
// 優先順位:
//  1. --api フラグで明示指定された URL または .marmot ファイルパス
//  2. $HOME/.marmot に登録されたアクティブエンドポイント
//     ($HOME/.marmot が無い場合は /etc/marmot/.marmot.example からコピーして自動作成)
func getClientConfig() (*client.MarmotEndpoint, error) {
	return config.GetClientConfig2(apiConfigFilename)
}

// clearScreen はターミナル画面をクリアする。
func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

// runList はリスト表示関数 fn を実行する。-w フラグが指定されている場合は
// watchInterval 秒ごとに fn を繰り返し呼び出し、Ctrl+C で終了する。
func runList(fn func() error) error {
	if !watchMode {
		return fn()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	for {
		clearScreen()
		if err := fn(); err != nil {
			return err
		}
		select {
		case <-sigCh:
			return nil
		case <-time.After(time.Duration(watchInterval) * time.Second):
		}
	}
}
