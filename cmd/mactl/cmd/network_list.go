package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

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
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

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
			sort.Slice(data, func(i, j int) bool {
				return creationTime(data[i].Status).Before(creationTime(data[j].Status))
			})
			if len(data) == 0 {
				fmt.Println("No networks found.")
				return nil
			}

			switch outputStyle {
			case "text":
				fmt.Print(formatNetworkListText(data))
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
		})
	},
}

func formatNetworkListText(data []api.VirtualNetwork) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "  %2s  %-10s  %-20s  %-12s  %-20s  %-20s\n", "No", "NETWORK-ID", "NETWORK-NAME", "NODE-NAME", "BRIDGE-NAME", "STATUS")
	for i, network := range data {
		fmt.Fprintf(&builder, "  %2d  %-10v  %-20v  %-12v  %-20v  %-20v\n",
			i+1,
			network.Id,
			stringValue(network.Metadata, func(m *api.Metadata) *string { return m.Name }),
			stringValue(network.Metadata, func(m *api.Metadata) *string { return m.NodeName }),
			stringValue(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.BridgeName }),
			networkListStatusLabel(network.Status),
		)
	}

	return builder.String()
}

func networkListStatusLabel(status *api.Status) string {
	if status == nil || status.Status == nil {
		return "N/A"
	}
	if name, ok := db.NetworkStatus[status.StatusCode]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN(%d)", status.StatusCode)
}

func init() {
	networkCmd.AddCommand(networkListCmd)
}
