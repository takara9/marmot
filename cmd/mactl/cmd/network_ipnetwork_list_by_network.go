package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var networkIPNetworkListByNetworkCmd = &cobra.Command{
	Use:     "ipn-by-vn [network id]",
	Aliases: []string{"ipnets-by-net", "ipnetwork-list-by-network"},
	Short:   "List IP networks under the specified virtual network",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

			for _, networkId := range args {
				byteBody, _, err := m.GetIpNetworksByVirtualNetworkId(networkId)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "GetIpNetworksByVirtualNetworkId", "Id", networkId, "err", err)
					continue
				}

				var data []api.IPNetwork
				switch outputStyle {
				case "text":
					if string(byteBody) == "null\n" {
						fmt.Println("IPネットワークが見つかりません。", "Network", networkId)
						continue
					}
					if err := json.Unmarshal(byteBody, &data); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", networkId, "err", err)
						continue
					}
					fmt.Println("VirtualNetwork:", networkId)
					fmt.Printf("  %2s  %-10s  %-18s  %-15s  %-15s\n", "No", "IPNET-ID", "ADDRESS/MASK", "START", "END")
					for i, n := range data {
						fmt.Printf("  %2d  %-10s", i+1, n.Id)
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
					continue

				case "json":
					var v interface{}
					if err := json.Unmarshal(byteBody, &v); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", networkId, "err", err)
						continue
					}
					formatted, err := json.MarshalIndent(v, "", "  ")
					if err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", networkId, "err", err)
						continue
					}
					fmt.Println(string(formatted))
					continue

				case "yaml":
					var v interface{}
					if err := json.Unmarshal(byteBody, &v); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", networkId, "err", err)
						continue
					}
					yamlBytes, err := yaml.Marshal(v)
					if err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", networkId, "err", err)
						continue
					}
					fmt.Println(string(yamlBytes))
					continue

				default:
					fmt.Fprintln(cmd.ErrOrStderr(), "output style must set text/json/yaml")
					continue
				}
			}
			return nil
		})
	},
}

func init() {
	networkCmd.AddCommand(networkIPNetworkListByNetworkCmd)
}
