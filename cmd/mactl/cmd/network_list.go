package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
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

		var data []api.VirtualNetwork
		if err := json.Unmarshal(byteBody, &data); err != nil {
			println("Failed to Unmarshal", err)
			return err
		}
		if len(data) == 0 {
			fmt.Println("No networks found.")
			return nil
		}

		switch outputStyle {
		case "text":
			for i, network := range data {
				fmt.Printf("  %2d", i+1)
				fmt.Printf("  %-10v", network.Id)
				fmt.Printf("  %-20v", *network.Metadata.Name)
				fmt.Printf("  %-20v", *network.Spec.BridgeName)
				fmt.Printf("  %-20v", db.NetworkStatus[*network.Status.Status])
				fmt.Println()
			}
			return nil

		case "json":
			byteBody, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				fmt.Println("Failed to Marshal", err)
			}
			fmt.Println(string(byteBody))
			return nil

		case "yaml":
			var data []config.VirtualNetwork
			if err := yaml.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				fmt.Println("Error:", err)
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
