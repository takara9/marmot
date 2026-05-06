package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"go.yaml.in/yaml/v3"
)

type legacyVirtualNetworkYAML struct {
	Spec *struct {
		IPNetworkAddress *string `yaml:"IPNetworkAddress,omitempty"`
	} `yaml:"Spec,omitempty"`
}

//var configFilename string

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new network",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		var conf api.VirtualNetwork
		err = config.ReadYamlConfig(configFilename, &conf)
		if err != nil {
			println("ReadYamlConfig", "err", err)
			return err
		}

		// 後方互換: 旧フォーマットの Spec.IPNetworkAddress も受け付ける
		if conf.Spec != nil && conf.Spec.IPNetworkAddress == nil {
			var legacy legacyVirtualNetworkYAML
			if err := config.ReadYamlConfig(configFilename, &legacy); err == nil {
				if legacy.Spec != nil && legacy.Spec.IPNetworkAddress != nil {
					conf.Spec.IPNetworkAddress = legacy.Spec.IPNetworkAddress
				}
			}
		}

		// 名前は必須項目
		if conf.Metadata == nil || conf.Metadata.Name == nil || len(*conf.Metadata.Name) == 0 {
			fmt.Println("Name is required in the configuration")
			return fmt.Errorf("name is required in the configuration")
		}

		byteBody, _, err := m.CreateVirtualNetwork(conf)
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
			fmt.Printf("ネットワークの作成要求が受け入れられました。ID: %v\n", networkMap["id"])
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
	networkCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "network.yaml", "Configuration file or raw URL for the network")
}
