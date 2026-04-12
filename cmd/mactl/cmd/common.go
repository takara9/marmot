package cmd

import (
	"fmt"
	"log/slog"
	"net/url"
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

// コンフィグからエンドポイントを取り出してセットする
//
// 優先順位:
//  1. --api フラグで明示指定された URL
//  2. $HOME/.marmot に登録されたアクティブエンドポイント
//     ($HOME/.marmot が無い場合は /etc/marmot/.marmot.example からコピーして自動作成)
func getClientConfig() (*client.MarmotEndpoint, error) {
	var rawURL string

	if len(apiConfigFilename) > 0 {
		// --api フラグで URL が直接指定された場合
		rawURL = apiConfigFilename
		// URL形式でなければ旧来のファイルパスとして扱う
		if u, err := url.Parse(apiConfigFilename); err == nil && u.Scheme != "" && u.Host != "" {
			rawURL = apiConfigFilename
		} else {
			// ファイルパスとして読み込む（後方互換）
			err := config.ReadYamlConfig(apiConfigFilename, &mactlConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
			rawURL = mactlConfig.ApiServerUrl
		}
	} else {
		// $HOME/.marmot を保証し（なければサンプルからコピー）、読み込む
		if err := config.EnsureMarmotConfig(); err != nil {
			return nil, err
		}
		marmotCfgPath := config.MarmotConfigPath()
		marmotCfg, err := config.ReadMarmotConfig(marmotCfgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", marmotCfgPath, err)
		}
		activeURL, err := marmotCfg.ActiveEndpoint()
		if err != nil {
			return nil, err
		}
		rawURL = activeURL
	}

	if len(rawURL) == 0 {
		rawURL = "http://localhost:8750"
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		slog.Error("mactlConfig", "read error", err)
		return nil, err
	}

	return client.NewMarmotdEp(
		u.Scheme,
		u.Host,
		"/api/v1",
		60,
	)
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
