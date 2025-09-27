package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmot"
)

// コンフィグからエンドポイントを取り出してセットする
func getClientConfig() (*marmot.MarmotEndpoint, error) {
	configFn := apiConfig
	if len(configFn) == 0 {
		configFn = filepath.Join(os.Getenv("HOME"), ".config_marmot")
	}
	config.ReadConfig(configFn, &cliConfig)
	if len(cliConfig.ApiServerUrl) == 0 {
		cliConfig.ApiServerUrl = "http://localhost:8080"
	}
	u, err := url.Parse(cliConfig.ApiServerUrl)
	if err != nil {
		return nil, err
	}

	return marmot.NewMarmotdEp(
		u.Scheme,
		u.Host,
		"/api/v1",
		60,
	)
}

// Ansible Playbook の適用
func apply_playbook(cnf config.MarmotConfig) {
	for {
		out, err := exec.Command("ansible", "-i", "hosts_kvm", "-m", "ping", "all").Output()
		if err != nil {
			fmt.Println("待機中...")
			time.Sleep(500 * time.Millisecond)
		} else {
			fmt.Println(string(out))
			break
		}
	}

	for _, spec := range cnf.VMSpec {
		path := fmt.Sprintf("playbook/%v", spec.AnsiblePB)
		out, err := exec.Command("ansible-playbook", "-i", "hosts_kvm", path).Output()
		if err != nil {
			fmt.Println("err = ", err)
		} else {
			fmt.Println(string(out))
		}
	}
}
