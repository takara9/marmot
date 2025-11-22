package cmd

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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

	//	var cnf api.MarmotConfig
	config.ReadConfig(configFn, &mactlConfig)
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

/*
// 新APIから旧APIの構造体へ変換する
func PrintMarmotConfig(a api.MarmotConfig) {
	fmt.Println("=========================================XX")
	if a.ClusterName != nil {
		fmt.Println("a.ClusterName=", *a.ClusterName)
	}
	if a.Domain != nil {
		fmt.Println("a.Domain=", *a.Domain)
	}
	if a.Hypervisor != nil {
		fmt.Println("a.Hypervisor=", *a.Hypervisor)
	}
	if a.ImageDefaultPath != nil {
		fmt.Println("a.ImageDefaultPath=", *a.ImageDefaultPath)
	}
	if a.ImgaeTemplatePath != nil {
		fmt.Println("a.ImgaeTemplatePath=", *a.ImgaeTemplatePath)
	}
	if a.NetDevDefault != nil {
		fmt.Println("a.NetDevDefault=", *a.NetDevDefault)
	}
	if a.NetDevPrivate != nil {
		fmt.Println("a.NetDevPrivate=", *a.NetDevPrivate)
	}
	if a.OsVariant != nil {
		fmt.Println("a.OsVariant=", *a.OsVariant)
	}
	if a.PrivateIpSubnet != nil {
		fmt.Println("a.PrivateIpSubnet=", *a.PrivateIpSubnet)
	}
	if a.PublicIpDns != nil {
		fmt.Println("a.PublicIpDns=", *a.PublicIpDns)
	}
	if a.PublicIpGw != nil {
		fmt.Println("a.PublicIpGw=", *a.PublicIpGw)
	}
	if a.PublicIpSubnet != nil {
		fmt.Println("a.PublicIpSubnet=", *a.PublicIpSubnet)
	}
	if a.Qcow2Image != nil {
		fmt.Println("a.Qcow2Image=", *a.Qcow2Image)
	}

	// ここでエラーになるのは、クライアントライブラリが未完のため
	if a.VmSpec != nil {
		for _, v := range *a.VmSpec {

			if v.Name != nil {
				fmt.Println("v.Name=", *v.Name)
			}
			if v.Cpu != nil {
				fmt.Println("v.Cpu=", int(*v.Cpu))
			}
			if v.Memory != nil {
				fmt.Println("v.Memory=", *v.Memory)
			}
			if v.PrivateIp != nil {
				fmt.Println("v.PrivateIp=", *v.PrivateIp)
			}
			if v.PublicIp != nil {
				fmt.Println("v.PublicIp=", *v.PublicIp)
			}
			if v.Comment != nil {
				fmt.Println("v.Comment=", *v.Comment)
			}
			if v.Key != nil {
				fmt.Println("v.Key=", *v.Key)
			}
			if v.Ostemplv != nil {
				fmt.Println("v.Ostemplv=", *v.Ostemplv)
			}
			if v.Ostempvg != nil {
				fmt.Println("v.Ostempvg=", *v.Ostempvg)
			}
			if v.Ostempvariant != nil {
				fmt.Println("v.Ostempvariant=", *v.Ostempvariant)
			}
			if v.Uuid != nil {
				fmt.Println("v.Uuid=", *v.Uuid)
			}
			if v.Playbook != nil {
				fmt.Println("v.Playbook=", *v.Playbook)
			}
			if v.Storage != nil {
				for _, v2 := range *v.Storage {
					if v2.Name != nil {
						fmt.Println("v2.Name=", *v2.Name)
					}
					if v2.Path != nil {
						fmt.Println("v2.Path=", *v2.Path)
					}
					if v2.Size != nil {
						fmt.Println("v2.Size=", int(*v2.Size))
					}
					if v2.Type != nil {
						fmt.Println("v2.Type=", *v2.Type)
					}
					if v2.Vg != nil {
						fmt.Println("v2.Vg=", *v2.Vg)
					}
				}
			}
		}
	}
	fmt.Println("=========================================XX")
}
*/
