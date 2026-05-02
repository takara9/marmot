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

var networkListShowAll bool

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

			data = filterNetworksForList(data, networkListShowAll)

			sort.SliceStable(data, func(i, j int) bool {
				ti := creationTime(data[i].Status)
				tj := creationTime(data[j].Status)

				hasI := !ti.IsZero()
				hasJ := !tj.IsZero()
				if hasI != hasJ {
					// creationTimeStamp が無いデータは末尾に寄せる。
					return hasI
				}

				if !ti.Equal(tj) {
					return ti.Before(tj)
				}

				return data[i].Id < data[j].Id
			})
			if len(data) == 0 {
				switch outputStyle {
				case "text":
					fmt.Println("No networks found.")
					return nil
				case "json":
					fmt.Println("[]")
					return nil
				case "yaml":
					fmt.Println("[]")
					return nil
				default:
					fmt.Println("output style must set text/json/yaml")
					return fmt.Errorf("output style must set text/json/yaml")
				}
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
				jsonBytes, err := json.Marshal(data)
				if err != nil {
					fmt.Println("Failed to Marshal", err)
					return err
				}
				var data []config.VirtualNetwork
				if err := yaml.Unmarshal(jsonBytes, &data); err != nil {
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

func filterHeadSyncRoleNetworks(data []api.VirtualNetwork) []api.VirtualNetwork {
	filtered := make([]api.VirtualNetwork, 0, len(data))
	for _, network := range data {
		if network.Metadata == nil || network.Metadata.Labels == nil {
			continue
		}
		if db.GetNetworkSyncRole(*network.Metadata.Labels) == "head" {
			filtered = append(filtered, network)
		}
	}
	return filtered
}

func filterNetworksForList(data []api.VirtualNetwork, showAll bool) []api.VirtualNetwork {
	if showAll {
		return data
	}
	return filterHeadSyncRoleNetworks(data)
}

func formatNetworkListText(data []api.VirtualNetwork) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "  %2s  %1s%-10s  %-20s  %-12s  %-20s  %-18s  %-20s\n", "No", "", "NETWORK-ID", "NETWORK-NAME", "NODE-NAME", "BRIDGE-NAME", "IP-NET", "STATUS")
	for i, network := range data {
		fmt.Fprintf(&builder, "  %2d  %1s%-10v  %-20v  %-12v  %-20v  %-18v  %-20v\n",
			i+1,
			deletionMarker(network.Status),
			network.Id,
			stringValue(network.Metadata, func(m *api.Metadata) *string { return m.Name }),
			stringValue(network.Metadata, func(m *api.Metadata) *string { return m.NodeName }),
			stringValue(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.BridgeName }),
			stringValue(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.IPNetworkAddress }),
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
	networkListCmd.Flags().BoolVarP(&networkListShowAll, "all", "a", false, "head/follower を含めてすべてのネットワークを表示する")
}
