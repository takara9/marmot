package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var networkDeleteCmd = &cobra.Command{
	Use:   "delete [network-id...]",
	Short: "Delete one or more networks",
	Args:  cobra.MinimumNArgs(1), // Idの列挙を許容
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, networkId := range args {
			byteBody, _, err := m.DeleteVirtualNetworkById(networkId)
			if err != nil {
				println("DeleteVirtualNetworkById", "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", err)
					return err
				}
				fmt.Println("ネットワークが削除されました。ID:", data.(map[string]interface{})["id"])
				continue

			case "json":
				fmt.Println(string(byteBody))
				continue

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
				continue

			default:
				fmt.Println("output style must set text/json/yaml")
				return fmt.Errorf("output style must set text/json/yaml")
			}
		}
		return nil
	},
}

func init() {
	networkCmd.AddCommand(networkDeleteCmd)
}
