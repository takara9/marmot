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
		var spec api.Server
		var volSpec api.Volume

		// 名前は必須項目
		if len(conf.Name) == 0 {
			fmt.Println("Name is required in the configuration")
			return fmt.Errorf("name is required in the configuration")
		}
		spec.Name = util.StringPtr(conf.Name)

		// 無設定を許容、デフォルトをAPI側に任せる
		if conf.Cpu != nil {
			spec.Cpu = util.IntPtrInt(*conf.Cpu)
		}

		if conf.Memory != nil {
			spec.Memory = util.IntPtrInt(*conf.Memory)
		}

		if conf.OsVariant != nil {
			spec.OsVariant = util.StringPtr(*conf.OsVariant)
		}

		if conf.BootVolume != nil {
			volSpec.Name = util.StringPtr("boot")
			volSpec.Type = util.StringPtr(*conf.BootVolume.Type)
		}
		spec.BootVolume = &volSpec
		spec.Network = &[]api.Network{}
		if conf.Network != nil {
			for _, nic := range *conf.Network {
				var n api.Network
				n.Id = nic.Name
				if nic.Address != nil {
					n.Address = util.StringPtr(*nic.Address)
					n.Dhcp4 = util.BoolPtr(false)
					n.Dhcp6 = util.BoolPtr(false)
				}
				if nic.Netmask != nil {
					n.Netmask = util.StringPtr(*nic.Netmask)
				}
				if nic.Routes != nil {
					routes := make([]api.Route, len(*nic.Routes))
					for i, r := range *nic.Routes {
						routes[i].To = util.StringPtr(r.Destination)
						routes[i].Via = util.StringPtr(r.Gateway)
					}
					n.Routes = &routes
				}

				if nic.Nameservers != nil {
					n.Nameservers = &api.Nameservers{}
					if nic.Nameservers.Addresses != nil {
						n.Nameservers.Addresses = &[]string{}
						for _, addr := range *nic.Nameservers.Addresses {
							fmt.Println("addr", addr)
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
				*spec.Network = append(*spec.Network, n)
			}
		}
		spec.Comment = conf.Comment

		if conf.Storage != nil {
			volumes := make([]api.Volume, len(*conf.Storage))
			for i, vol := range *conf.Storage {
				volumes[i].Name = util.StringPtr(vol.Name)
				volumes[i].Size = util.IntPtrInt(*vol.Size)
				volumes[i].Comment = util.StringPtr(*vol.Comment)
				if vol.Type == nil {
					volumes[i].Type = util.StringPtr("qcow2")
				} else {
					volumes[i].Type = util.StringPtr(*vol.Type)
				}
				if vol.Kind == nil {
					volumes[i].Kind = util.StringPtr("data")
				} else {
					volumes[i].Kind = util.StringPtr(*vol.Kind)
				}
			}
			spec.Storage = &volumes
		}

		byteBody, _, err := m.CreateServer(spec)
		if err != nil {
			println("CreateServer", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			println("Not implemented for text output")
			println(string(byteBody))
			return nil

		case "json":
			println(string(byteBody))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				println("Failed to Marshal", err)
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
	serverCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "vm-spec.yaml", "Configuration file for the server")
}
