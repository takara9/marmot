package config

import (
	"log/slog"
	"net/url"

	"github.com/takara9/marmot/pkg/client"
)

// GetClientConfig2 はコンフィグからエンドポイントを取り出して返す。
// apiConfigFilename が指定された場合はそのファイルを $HOME/.marmot 形式として読み込む。
// 省略時は EnsureMarmotConfig() で $HOME/.marmot を保証してから読み込む。
func GetClientConfig2(apiConfigFilename string) (*client.MarmotEndpoint, error) {
	var rawURL string

	if len(apiConfigFilename) > 0 {
		// ファイルパスとして .marmot フォーマットで読み込む
		cfg, err := ReadMarmotConfig(apiConfigFilename)
		if err != nil {
			slog.Error("GetClientConfig2", "read error", err)
			return nil, err
		}
		rawURL, err = cfg.ActiveEndpoint()
		if err != nil {
			slog.Error("GetClientConfig2", "active endpoint error", err)
			return nil, err
		}
	} else {
		// $HOME/.marmot を保証し読み込む
		if err := EnsureMarmotConfig(); err != nil {
			slog.Error("GetClientConfig2", "ensure config error", err)
			return nil, err
		}
		cfg, err := ReadMarmotConfig(MarmotConfigPath())
		if err != nil {
			slog.Error("GetClientConfig2", "read error", err)
			return nil, err
		}
		var aErr error
		rawURL, aErr = cfg.ActiveEndpoint()
		if aErr != nil {
			slog.Error("GetClientConfig2", "active endpoint error", aErr)
			return nil, aErr
		}
	}

	if len(rawURL) == 0 {
		rawURL = "http://localhost:8750"
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		slog.Error("GetClientConfig2", "parse error", err)
		return nil, err
	}

	return client.NewMarmotdEp(
		u.Scheme,
		u.Host,
		"/api/v1",
		60,
	)
}
