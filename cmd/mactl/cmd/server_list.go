package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

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

				fmt.Printf("  %2s  %-10s  %-20s  %-12s  %-3s  %-8s  %-12s  %-15s  %-15s\n", "No", "Server-ID", "Server-Name", "Status", "CPU", "RAM(MB)", "Node", "IP-Address", "Network")
				for i, server := range data {
					fmt.Printf("  %2d", i+1)
					fmt.Printf("  %-10v", server.Id)
					fmt.Printf("  %-20v", *server.Metadata.Name)
					fmt.Printf("  %-12v", db.ServerStatus[server.Status.StatusCode])
					fmt.Printf("  %-3v", *server.Spec.Cpu)
					fmt.Printf("  %-8v", *server.Spec.Memory)
					if server.Metadata.NodeName != nil {
						fmt.Printf("  %-12v", *server.Metadata.NodeName)
					} else {
						fmt.Printf("  %-12v", "N/A")
					}
					if server.Spec.NetworkInterface != nil {
						for j, nic := range *server.Spec.NetworkInterface {
							if j == 0 {
								if nic.Address != nil {
									fmt.Printf("  %-15v", *nic.Address)
									fmt.Printf("  %-15v", nic.Networkname)
								} else {
									fmt.Printf("  %-15v", "N/A")
									fmt.Printf("  %-15v", "N/A")
								}
							}
						}
					} else {
						fmt.Printf("  %-15s", "N/A")
					}
					fmt.Println()
				}
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

func init() {
	serverCmd.AddCommand(serverListCmd)
}
