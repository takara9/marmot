package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

			byteBody, _, err := m.GetServers()
			if err != nil {
				println("エラー応答が返されました。", "err", err)
				return nil
			}

			//var data interface{}
			var data []api.Server
			switch outputStyle {
			case "text":
				if string(byteBody) == "null\n" {
					fmt.Println("サーバーが見つかりません。")
					return nil
				}

				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", err)
					return err
				}
				sort.Slice(data, func(i, j int) bool {
					return creationTime(data[i].Status).Before(creationTime(data[j].Status))
				})

				fmt.Print(formatServerListText(data))
				return nil

			case "json":
				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", err)
					return err
				}
				sort.Slice(data, func(i, j int) bool {
					return creationTime(data[i].Status).Before(creationTime(data[j].Status))
				})
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
		})
	},
}

func formatServerListText(data []api.Server) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("  %2s  %-10s  %-20s  %-12s  %-3s  %-8s  %-12s  %-15s  %-15s\n", "No", "Server-ID", "Server-Name", "Status", "CPU", "RAM(MB)", "Node", "IP-Address", "Network"))
	for i, server := range data {
		networkLines := serverNetworkLines(server)
		builder.WriteString(fmt.Sprintf("  %2d  %-10v  %-20v  %-12v  %-3v  %-8v  %-12v  %-15v  %-15v\n",
			i+1,
			server.Id,
			serverDisplayName(server),
			serverStatusText(server),
			serverCPU(server),
			serverMemory(server),
			serverNodeName(server),
			networkLines[0].address,
			networkLines[0].network,
		))

		for _, networkLine := range networkLines[1:] {
			builder.WriteString(fmt.Sprintf("  %2s  %-10s  %-20s  %-12s  %-3s  %-8s  %-12s  %-15v  %-15v\n",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				networkLine.address,
				networkLine.network,
			))
		}
	}

	return builder.String()
}

type serverNetworkLine struct {
	address string
	network string
}

func serverNetworkLines(server api.Server) []serverNetworkLine {
	if server.Spec == nil || server.Spec.NetworkInterface == nil || len(*server.Spec.NetworkInterface) == 0 {
		return []serverNetworkLine{{address: "N/A", network: "N/A"}}
	}

	lines := make([]serverNetworkLine, 0, len(*server.Spec.NetworkInterface))
	for _, nic := range *server.Spec.NetworkInterface {
		address := "N/A"
		if nic.Address != nil && *nic.Address != "" {
			address = *nic.Address
		}

		network := nic.Networkname
		if network == "" {
			network = "N/A"
		}

		lines = append(lines, serverNetworkLine{address: address, network: network})
	}

	if len(lines) == 0 {
		return []serverNetworkLine{{address: "N/A", network: "N/A"}}
	}

	return lines
}

func serverDisplayName(server api.Server) string {
	if server.Metadata != nil && server.Metadata.Name != nil {
		return *server.Metadata.Name
	}
	return "N/A"
}

func serverNodeName(server api.Server) string {
	if server.Metadata != nil && server.Metadata.NodeName != nil {
		return *server.Metadata.NodeName
	}
	return "N/A"
}

func serverStatusText(server api.Server) string {
	if server.Status == nil {
		return "N/A"
	}
	return db.ServerStatus[server.Status.StatusCode]
}

func serverCPU(server api.Server) interface{} {
	if server.Spec != nil && server.Spec.Cpu != nil {
		return *server.Spec.Cpu
	}
	return "N/A"
}

func serverMemory(server api.Server) interface{} {
	if server.Spec != nil && server.Spec.Memory != nil {
		return *server.Spec.Memory
	}
	return "N/A"
}

func init() {
	serverCmd.AddCommand(serverListCmd)
}
