package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var networkIPAddressListCmd = &cobra.Command{
	Use:     "ips [ip network id]",
	Aliases: []string{"ipaddress-list"},
	Short:   "List allocated IP addresses in the specified IP network",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

			for _, ipNetworkId := range args {
				byteBody, _, err := m.GetIpAddressesByNetworkId(ipNetworkId)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "GetIpAddressesByNetworkId", "Id", ipNetworkId, "err", err)
					continue
				}

				var data []api.IPAddress
				switch outputStyle {
				case "text":
					if string(byteBody) == "null\n" {
						fmt.Println("IPアドレスが見つかりません。", "IpNetwork", ipNetworkId)
						continue
					}
					if err := json.Unmarshal(byteBody, &data); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", ipNetworkId, "err", err)
						continue
					}
					fmt.Println("IpNetwork:", ipNetworkId)
					fmt.Printf("  %2s  %-15s  %-15s  %-10s  %-10s\n", "No", "IP", "NETMASK", "HOST-ID", "NETWORK-ID")
					for i, a := range data {
						fmt.Printf("  %2d", i+1)
						if a.IPAddress != nil {
							fmt.Printf("  %-15s", *a.IPAddress)
						} else {
							fmt.Printf("  %-15s", "N/A")
						}
						if a.Netmask != nil {
							fmt.Printf("  %-15s", *a.Netmask)
						} else {
							fmt.Printf("  %-15s", "N/A")
						}
						if a.HostId != nil {
							fmt.Printf("  %-10s", *a.HostId)
						} else {
							fmt.Printf("  %-10s", "N/A")
						}
						if a.NetworkId != nil {
							fmt.Printf("  %-10s", *a.NetworkId)
						} else {
							fmt.Printf("  %-10s", "N/A")
						}
						fmt.Println()
					}
					continue

				case "json":
					var v interface{}
					if err := json.Unmarshal(byteBody, &v); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", ipNetworkId, "err", err)
						continue
					}
					formatted, err := json.MarshalIndent(v, "", "  ")
					if err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", ipNetworkId, "err", err)
						continue
					}
					fmt.Println(string(formatted))
					continue

				case "yaml":
					var v interface{}
					if err := json.Unmarshal(byteBody, &v); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", ipNetworkId, "err", err)
						continue
					}
					yamlBytes, err := yaml.Marshal(v)
					if err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", ipNetworkId, "err", err)
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
	networkCmd.AddCommand(networkIPAddressListCmd)
}
