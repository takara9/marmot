package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all networks",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		byteBody, _, err := m.GetVirtualNetworks()
		if err != nil {
			println("エラー応答が返されました。", "err", err)
			return nil
		}

		//var data interface{}
		var data []api.VirtualNetwork
		switch outputStyle {
		case "text":
			if string(byteBody) == "null\n" {
				fmt.Println("仮想ネットワークが見つかりません。")
				return nil
			}

			for i, network := range data {
				fmt.Printf("  %2d", i+1)
				fmt.Printf("  %-10v", network.Id)
				fmt.Printf("  %-20v", *network.Metadata.Name)
				fmt.Printf("  %-20v", *network.Spec.BridgeName)
				fmt.Println()
			}
			return nil

		case "json":
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			byteBody, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				fmt.Println("Failed to Marshal", err)
			}
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
	networkCmd.AddCommand(networkListCmd)
}
