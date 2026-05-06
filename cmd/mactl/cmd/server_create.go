package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/util"
	"go.yaml.in/yaml/v3"
)

var configFilename string

// resolveVolumeIdByName は名前でボリュームを検索し、一意に特定できた場合はそのIDを返す。
// 同名ボリュームが複数ある場合はエラーを返す。
func resolveVolumeIdByName(m *client.MarmotEndpoint, name string) (string, error) {
	byteBody, _, err := m.ListVolumes()
	if err != nil {
		return "", fmt.Errorf("ListVolumes failed: %w", err)
	}
	var volumes []api.Volume
	if err := json.Unmarshal(byteBody, &volumes); err != nil {
		return "", fmt.Errorf("failed to parse volume list: %w", err)
	}
	var matched []api.Volume
	for _, v := range volumes {
		if v.Metadata != nil && v.Metadata.Name != nil && *v.Metadata.Name == name {
			matched = append(matched, v)
		}
	}
	switch len(matched) {
	case 0:
		return "", fmt.Errorf("ボリューム名 %q に一致するボリュームが見つかりません", name)
	case 1:
		return matched[0].Id, nil
	default:
		return "", fmt.Errorf("ボリューム名 %q に一致するボリュームが複数存在します (%d件)", name, len(matched))
	}
}

var serverCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new server",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		var virtualServer api.Server
		err = config.ReadYamlConfig(configFilename, &virtualServer)
		if err != nil {
			return err
		}

		// Metadata.name は必須
		if virtualServer.Metadata == nil || virtualServer.Metadata.Name == nil || *virtualServer.Metadata.Name == "" {
			return fmt.Errorf("Metadata.name is required in the configuration")
		}

		if virtualServer.Spec != nil {
			// bootVolume の名前解決と既定値設定
			if virtualServer.Spec.BootVolume != nil {
				bv := virtualServer.Spec.BootVolume
				if bv.Id == "" && bv.Spec == nil && bv.Metadata != nil && bv.Metadata.Name != nil && *bv.Metadata.Name != "" {
					// id 未設定・Spec 無し・名前あり → 名前でボリューム検索
					pvId, err := resolveVolumeIdByName(m, *bv.Metadata.Name)
					if err != nil {
						fmt.Fprintln(os.Stderr, "bootVolume:", err)
						return err
					}
					bv.Id = pvId
					bv.Metadata.Id = util.StringPtr(pvId)
				} else if bv.Id == "" && bv.Spec != nil {
					// 新規ボリューム：名前未設定なら "boot" をデフォルトに
					if bv.Metadata == nil {
						bv.Metadata = &api.Metadata{Name: util.StringPtr("boot")}
					} else if bv.Metadata.Name == nil {
						bv.Metadata.Name = util.StringPtr("boot")
					}
				}
			}

			// Storage の名前解決とデフォルト値設定
			if virtualServer.Spec.Storage != nil {
				for i := range *virtualServer.Spec.Storage {
					vol := &(*virtualServer.Spec.Storage)[i]
					if vol.Id == "" && vol.Spec == nil && vol.Metadata != nil && vol.Metadata.Name != nil && *vol.Metadata.Name != "" {
						// id 未設定・Spec 無し・名前あり → 名前でボリューム検索
						pvId, err := resolveVolumeIdByName(m, *vol.Metadata.Name)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Storage[%d]: %v\n", i, err)
							return err
						}
						vol.Id = pvId
						if vol.Metadata != nil {
							vol.Metadata.Id = util.StringPtr(pvId)
						}
					} else if vol.Spec != nil {
						// 新規ボリューム：type/kind のデフォルト
						if vol.Spec.Type == nil {
							vol.Spec.Type = util.StringPtr("qcow2")
						}
						if vol.Spec.Kind == nil {
							vol.Spec.Kind = util.StringPtr("data")
						}
					}
				}
			}

			// NetworkInterface の後処理
			if virtualServer.Spec.NetworkInterface != nil {
				for i := range *virtualServer.Spec.NetworkInterface {
					nic := &(*virtualServer.Spec.NetworkInterface)[i]
					// address が設定されていて dhcp4 未設定なら false にする
					if nic.Address != nil && nic.Dhcp4 == nil {
						nic.Dhcp4 = util.BoolPtr(false)
						nic.Dhcp6 = util.BoolPtr(false)
					}
					// netmask が数値文字列（CIDR長）の場合、netmasklen に変換
					if nic.Netmasklen == nil && nic.Netmask != nil {
						if maskLen, err := strconv.Atoi(*nic.Netmask); err == nil && maskLen >= 0 && maskLen <= 128 {
							nic.Netmasklen = util.IntPtrInt(maskLen)
						}
					}
				}
			}
		}

		byteBody, _, err := m.CreateServer(virtualServer)
		if err != nil {
			fmt.Println("CreateServer", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Println("Failed to Unmarshal", err)
				return err
			}
			serverMap := data.(map[string]interface{})
			fmt.Printf("サーバーの作成要求が受け入れられました。ID: %v\n", serverMap["id"])
			return nil

		case "json":
			fmt.Println(string(byteBody))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Println("Failed to Unmarshal", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				fmt.Println("Failed to Marshal", err)
				return err
			}
			fmt.Println(string(yamlBytes))
			return nil

		default:
			fmt.Println("output style must set text/json/yaml")
			return fmt.Errorf("output style must set text/json/yaml")
		}

	},
}

func init() {
	serverCmd.AddCommand(serverCreateCmd)
	serverCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "vm-server.yaml", "Configuration file or raw URL for the server")
}
