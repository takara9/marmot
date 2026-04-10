package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MarmotConfig は $HOME/.marmot に保存される複数エンドポイント設定
type MarmotConfig struct {
	// Current はアクティブなエンドポイントのインデックス (0始まり)
	Current   int      `yaml:"current"`
	Endpoints []string `yaml:"endpoints"`
}

// MarmotConfigPath は $HOME/.marmot のパスを返す
func MarmotConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".marmot")
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
	return &cfg, nil
}

// WriteMarmotConfig は MarmotConfig を指定パスに書き込む
func WriteMarmotConfig(path string, cfg *MarmotConfig) error {
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
