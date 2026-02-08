package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/util"
	"go.yaml.in/yaml/v3"
)

var configFilename string

var serverCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new server",
	RunE: func(cmd *cobra.Command, args []string) error {
		var conf config.Server
		err := config.ReadYamlConfig(configFilename, &conf)
		if err != nil {
			println("ReadYamlConfig", "err", err)
			return err
		}

		// 名前は必須項目
		if len(conf.Name) == 0 {
			fmt.Println("Name is required in the configuration")
			return fmt.Errorf("name is required in the configuration")
		}

		var virtualServer api.Server
		virtualServer.Metadata = &api.Metadata{
			Name: util.StringPtr(conf.Name),
		}
		virtualServer.Spec = &api.VmSpec{}
		virtualServer.Spec.BootVolume = &api.Volume{}
		virtualServer.Spec.Storage = &[]api.Volume{}
		virtualServer.Spec.BootVolume.Spec = &api.VolSpec{}
		virtualServer.Spec.BootVolume.Metadata = &api.Metadata{}
		virtualServer.Spec.Network = &[]api.Network{}

		if conf.Comment != nil {
			virtualServer.Metadata.Comment = util.StringPtr(*conf.Comment)
		}
		// 無設定を許容、デフォルトをAPI側に任せる
		if conf.Cpu != nil {
			virtualServer.Spec.Cpu = util.IntPtrInt(*conf.Cpu)
		}

		if conf.Memory != nil {
			virtualServer.Spec.Memory = util.IntPtrInt(*conf.Memory)
		}

		if conf.OsVariant != nil {
			virtualServer.Spec.OsVariant = util.StringPtr(*conf.OsVariant)
		}

		if conf.BootVolume != nil {
			virtualServer.Spec.BootVolume.Metadata.Name = util.StringPtr("boot")
			virtualServer.Spec.BootVolume.Spec.Type = util.StringPtr(*conf.BootVolume.Type)
			if conf.BootVolume.Size != nil {
				virtualServer.Spec.BootVolume.Spec.Size = util.IntPtrInt(*conf.BootVolume.Size)
			}
		}

		if conf.Network != nil {
			for _, nic := range *conf.Network {
				// ネットワークの設定があるときの追加設定
				var n api.Network
				n.Id = nic.Name // NameをIDとして使用
				// 設定があれば固定IP設定
				if nic.Address != nil {
					n.Address = util.StringPtr(*nic.Address)
					n.Dhcp4 = util.BoolPtr(false)
					n.Dhcp6 = util.BoolPtr(false)
				}
				// 設定があればネットマスク設定
				if nic.Netmask != nil {
					n.Netmask = util.StringPtr(*nic.Netmask)
				}
				// 設定があればルート設定
				if nic.Routes != nil {
					routes := make([]api.Route, len(*nic.Routes))
					for i, r := range *nic.Routes {
						routes[i].To = util.StringPtr(r.Destination)
						routes[i].Via = util.StringPtr(r.Gateway)
					}
					n.Routes = &routes
				}

				// VLANの設定があれば対応
				if nic.Portgroup != nil {
					n.Portgroup = util.StringPtr(*nic.Portgroup)
				}
				// VLANの設定があれば対応
				if nic.Vlans != nil {
					vlans := make([]uint, len(*nic.Vlans))
					for i, v := range *nic.Vlans {
						vlans[i] = uint(v)
					}
					n.Vlans = &vlans
				}
				// ネームサーバーの設定があれば対応
				if nic.Nameservers != nil {
					n.Nameservers = &api.Nameservers{}
					if nic.Nameservers.Addresses != nil {
						n.Nameservers.Addresses = &[]string{}
						for _, addr := range *nic.Nameservers.Addresses {
							*n.Nameservers.Addresses = append(*n.Nameservers.Addresses, addr)
						}
					}
					if nic.Nameservers.Search != nil {
						n.Nameservers.Search = &[]string{}
						for _, search := range *nic.Nameservers.Search {
							*n.Nameservers.Search = append(*n.Nameservers.Search, search)
						}
					}
				}
				// インターフェースの設定追加
				*virtualServer.Spec.Network = append(*virtualServer.Spec.Network, n)
			}
		}

		// ストレージの追加設定
		if conf.Storage != nil {
			volumes := make([]api.Volume, len(*conf.Storage))
			for i, vol := range *conf.Storage {
				meta := api.Metadata{}
				volumes[i].Metadata = &meta
				volumes[i].Metadata.Name = util.StringPtr(vol.Name)
				volumes[i].Metadata.Comment = util.StringPtr(*vol.Comment)
				spec := api.VolSpec{}
				volumes[i].Spec = &spec
				volumes[i].Spec.Size = util.IntPtrInt(*vol.Size)
				if vol.Type == nil {
					volumes[i].Spec.Type = util.StringPtr("qcow2")
				} else {
					volumes[i].Spec.Type = util.StringPtr(*vol.Type)
				}
				if vol.Kind == nil {
					volumes[i].Spec.Kind = util.StringPtr("data")
				} else {
					volumes[i].Spec.Kind = util.StringPtr(*vol.Kind)
				}
			}
			virtualServer.Spec.Storage = &volumes
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
			fmt.Printf("サーバーが作成されました。ID: %v\n", serverMap["id"])
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
	serverCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "vm-server.yaml", "Configuration file for the server")
}
