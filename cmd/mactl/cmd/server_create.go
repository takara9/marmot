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
		spec.Name = util.StringPtr(conf.Name)
		spec.Cpu = util.IntPtrInt(*conf.Cpu)
		spec.Memory = util.IntPtrInt(*conf.Memory)
		if conf.Nic != nil {
			for i, nic := range *conf.Nic {
				if i == 0 {
					if nic.IpAddress != nil {
						spec.PrivateIp = util.StringPtr(*nic.IpAddress)
					}
				}
				if i == 1 {
					if nic.IpAddress != nil {
						spec.PublicIp = util.StringPtr(*nic.IpAddress)
					}
				}
			}
		}
		spec.Comment = conf.Comment

		if conf.Storage != nil {
			volumes := make([]api.Volume, len(*conf.Storage))
			for i, vol := range *conf.Storage {
				volumes[i].Name = util.StringPtr(vol.Name)
				volumes[i].Size = util.IntPtrInt(*vol.Size)
				volumes[i].Comment = util.StringPtr(*vol.Comment)
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
