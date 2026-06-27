package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// defaultMarmotConfig は ~/.marmot が存在しない場合に書き込むデフォルト設定内容
const defaultMarmotConfig = `current: 0
endpoints:
  - https://localhost:8750
insecure-skip-tls-verify: false
`

// EnsureMarmotConfig は $HOME/.marmot が存在しない場合、
// デフォルト設定を直接書き込んで自動作成する。
func EnsureMarmotConfig() error {
	dest := MarmotConfigPath()
	if _, err := os.Stat(dest); err == nil {
		return nil // すでに存在する
	}
	if err := os.WriteFile(dest, []byte(defaultMarmotConfig), 0600); err != nil {
		return fmt.Errorf("設定ファイルの作成に失敗しました (%s): %w", dest, err)
	}
	slog.Debug("設定ファイルを作成しました", "path", dest)
	return nil
}

// MarmotConfig は $HOME/.marmot に保存される複数エンドポイント設定
type MarmotConfig struct {
	// Current はアクティブなエンドポイントのインデックス (0始まり)
	Current          int      `yaml:"current"`
	Endpoints        []string `yaml:"endpoints"`
	InsecureSkipTLSVerify bool `yaml:"insecure-skip-tls-verify"`
	EndpointComments []string `yaml:"endpointComments,omitempty"`
}

// MarmotConfigPath は $HOME/.marmot のパスを返す。
// Windows では USERPROFILE を HOME の代わりに使う。
func MarmotConfigPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return filepath.Join(home, ".marmot")
}

// ReadMarmotConfig は指定パスから MarmotConfig を読み込む
func ReadMarmotConfig(path string) (*MarmotConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg MarmotConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	cfg.NormalizeEndpointComments()
	return &cfg, nil
}

// WriteMarmotConfig は MarmotConfig を指定パスに書き込む
func WriteMarmotConfig(path string, cfg *MarmotConfig) error {
	cfg.NormalizeEndpointComments()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	// 0600 で書き込む（API URLに認証情報が含まれる可能性があるため）
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// NormalizeEndpointComments は Endpoints と EndpointComments の要素数を揃える。
func (c *MarmotConfig) NormalizeEndpointComments() {
	if len(c.EndpointComments) > len(c.Endpoints) {
		c.EndpointComments = c.EndpointComments[:len(c.Endpoints)]
		return
	}
	for len(c.EndpointComments) < len(c.Endpoints) {
		c.EndpointComments = append(c.EndpointComments, "")
	}
}

// EndpointComment は指定インデックスのコメントを返す。
func (c *MarmotConfig) EndpointComment(index int) string {
	if index < 0 || index >= len(c.EndpointComments) {
		return ""
	}
	return c.EndpointComments[index]
}

// ActiveEndpoint は現在アクティブなエンドポイントURLを返す
func (c *MarmotConfig) ActiveEndpoint() (string, error) {
	if len(c.Endpoints) == 0 {
		return "", fmt.Errorf("エンドポイントが登録されていません。'mactl ep add <URL>' で追加してください")
	}
	if c.Current < 0 || c.Current >= len(c.Endpoints) {
		return "", fmt.Errorf("無効なエンドポイント番号: %d (登録数: %d)", c.Current, len(c.Endpoints))
	}
	return c.Endpoints[c.Current], nil
}
