package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var networkIPNetworkListCmd = &cobra.Command{
	Use:     "ipn",
	Aliases: []string{"ipnets", "ipnetwork-list"},
	Short:   "List all assigned IP networks",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

			byteBody, _, err := m.ListIpNetworks()
			if err != nil {
				println("エラー応答が返されました。", "err", err)
				return nil
			}

			var data []api.IPNetwork
			switch outputStyle {
			case "text":
				if string(byteBody) == "null\n" {
					fmt.Println("IPネットワークが見つかりません。")
					return nil
				}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					fmt.Println("Failed to Unmarshal", err)
					return err
				}
				fmt.Printf("  %2s  %-10s  %-10s  %-18s  %-15s  %-15s\n", "No", "IPNET-ID", "VNET-ID", "ADDRESS/MASK", "START", "END")
				for i, n := range data {
					fmt.Printf("  %2d  %-10s", i+1, n.Id)
					if n.VirtualNetworkId != nil {
						fmt.Printf("  %-10s", *n.VirtualNetworkId)
					} else {
						fmt.Printf("  %-10s", "N/A")
					}
					if n.AddressMaskLen != nil {
						fmt.Printf("  %-18s", *n.AddressMaskLen)
					} else {
						fmt.Printf("  %-18s", "N/A")
					}
					if n.StartAddress != nil {
						fmt.Printf("  %-15s", *n.StartAddress)
					} else {
						fmt.Printf("  %-15s", "N/A")
					}
					if n.EndAddress != nil {
						fmt.Printf("  %-15s", *n.EndAddress)
					} else {
						fmt.Printf("  %-15s", "N/A")
					}
					fmt.Println()
				}
				return nil

			case "json":
				if err := json.Unmarshal(byteBody, &data); err != nil {
					fmt.Println("Failed to Unmarshal", err)
					return err
				}
				byteBody, err := json.MarshalIndent(data, "", "  ")
				if err != nil {
					fmt.Println("Failed to Marshal", err)
					return err
				}
				fmt.Println(string(byteBody))
				return nil

			case "yaml":
				var v interface{}
				if err := json.Unmarshal(byteBody, &v); err != nil {
					fmt.Println("Failed to Unmarshal", err)
					return err
				}
				yamlBytes, err := yaml.Marshal(v)
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
		})
	},
}

func init() {
	networkCmd.AddCommand(networkIPNetworkListCmd)
}
