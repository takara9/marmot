package config

import (
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"github.com/takara9/marmot/pkg/client"
)

const DefaultTimeoutSeconds = 60

// コンフィグからエンドポイントを取り出してセットする
func GetClientConfig2(apiConfigFilename string) (*client.MarmotEndpoint, error) {
	var configFn string
	if len(apiConfigFilename) == 0 {
		configFn = filepath.Join(os.Getenv("HOME"), ".config_marmot")
	} else {
		configFn = apiConfigFilename
	}

	var mactlConfig DefaultConfig
	//	var cnf api.MarmotConfig
	if ReadConfig(configFn, &mactlConfig) != nil {
		slog.Error("mactlConfig", "read error", nil)
		return nil, nil
	}	
	if len(mactlConfig.ApiServerUrl) == 0 {
		mactlConfig.ApiServerUrl = "http://localhost:8080"
	}

	u, err := url.Parse(mactlConfig.ApiServerUrl)
	if err != nil {
		slog.Error("mactlConfig", "read error", err)
		return nil, err
	}

	return client.NewMarmotdEp(
		u.Scheme,
		u.Host,
		"/api/v1",
		DefaultTimeoutSeconds,
	)
}
