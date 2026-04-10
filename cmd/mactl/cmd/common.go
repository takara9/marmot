package cmd

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
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
//  3. フォールバック: $HOME/.config_marmot (後方互換)
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
		// $HOME/.marmot が存在すれば優先的に使用する
		marmotCfgPath := config.MarmotConfigPath()
		if marmotCfg, err := config.ReadMarmotConfig(marmotCfgPath); err == nil {
			activeURL, err := marmotCfg.ActiveEndpoint()
			if err != nil {
				return nil, err
			}
			rawURL = activeURL
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read %s: %w", marmotCfgPath, err)
		} else {
			// フォールバック: 旧来の .config_marmot を読み込む
			configFn := filepath.Join(os.Getenv("HOME"), ".config_marmot")
			if err := config.ReadYamlConfig(configFn, &mactlConfig); err != nil {
				return nil, fmt.Errorf("設定ファイルが見つかりません。'mactl ep add <URL>' または ~/.config_marmot を作成してください: %w", err)
			}
			rawURL = mactlConfig.ApiServerUrl
		}
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
