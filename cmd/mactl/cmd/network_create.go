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

//var configFilename string

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new network",
	RunE: func(cmd *cobra.Command, args []string) error {
		var conf config.VirtualNetwork
		err := config.ReadYamlConfig(configFilename, &conf)
		if err != nil {
			println("ReadYamlConfig", "err", err)
			return err
		}

		// 名前は必須項目
		if len(*conf.Metadata.Name) == 0 {
			fmt.Println("Name is required in the configuration")
			return fmt.Errorf("name is required in the configuration")
		}

		var virtualNetwork *api.VirtualNetwork
		virtualNetwork, err = util.ConvertToJSON(&conf)
		if err != nil {
			fmt.Println("ConvertToJSON", "err", err)
			return err
		}

		byteBody, _, err := m.CreateVirtualNetwork(*virtualNetwork)
		if err != nil {
			fmt.Println("CreateVirtualNetwork", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Println("Failed to Unmarshal", err)
				return err
			}
			networkMap := data.(map[string]interface{})
			fmt.Printf("ネットワークが作成されました。ID: %v\n", networkMap["id"])
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
	networkCmd.AddCommand(networkCreateCmd)
	networkCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "network.yaml", "Configuration file for the network")
}
