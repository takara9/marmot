package cmd

import (
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/config"
)

// コンフィグからエンドポイントを取り出してセットする
func getClientConfig() (*client.MarmotEndpoint, error) {
	var configFn string
	if len(apiConfigFilename) == 0 {
		configFn = filepath.Join(os.Getenv("HOME"), ".config_marmot")
	} else {
		configFn = apiConfigFilename
	}

	config.ReadYamlConfig(configFn, &mactlConfig)
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
		60,
	)
}

/*
// Ansible Playbook の適用
func apply_playbook(cnf config.MarmotConfig) {
	for {
		out, err := exec.Command("ansible", "-i", "hosts_kvm", "-m", "ping", "all").Output()
		if err != nil {
			slog.Error("待機中", "...", "")
			time.Sleep(500 * time.Millisecond)
		} else {
			fmt.Println(string(out))
			break
		}
	}

	for _, spec := range *cnf.VmSpec {
		path := fmt.Sprintf("playbook/%v", *spec.Playbook)
		out, err := exec.Command("ansible-playbook", "-i", "hosts_kvm", path).Output()
		if err != nil {
			slog.Error("問題発生", "err", err)
		} else {
			fmt.Println(string(out))
		}
	}
}
*/
