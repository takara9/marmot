package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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

// accessTokenPath returns the path to the access token file
func accessTokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".marmot", "token"), nil
}

// saveAccessToken saves the access token to a file
func saveAccessToken(token string) error {
	tokenPath, err := accessTokenPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}
	return nil
}

// loadAccessToken loads the access token from file
func loadAccessToken() (string, error) {
	tokenPath, err := accessTokenPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// clearAccessToken removes the access token file
func clearAccessToken() error {
	tokenPath, err := accessTokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear token: %w", err)
	}
	return nil
}

// loadTokenForEndpoint loads and sets the access token for the endpoint
func loadTokenForEndpoint(m *client.MarmotEndpoint) error {
	token, err := loadAccessToken()
	if err != nil {
		return err
	}
	if token != "" {
		m.SetAccessToken(token)
	}
	return nil
}
