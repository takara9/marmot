package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var marmotStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show marmot host status",
	Long:  `Show the status of the marmot host node, including resource capacity and current allocation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

			byteBody, _, err := m.GetMarmotStatus()
			if err != nil {
				fmt.Fprintln(os.Stderr, "エラー応答が返されました。", err)
				return nil
			}

			var status api.HostStatus
			switch outputStyle {
			case "text":
				if err := json.Unmarshal(byteBody, &status); err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Unmarshal", err)
					return err
				}
				printHostStatus(status)
				return nil

			case "json":
				if err := json.Unmarshal(byteBody, &status); err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Unmarshal", err)
					return err
				}
				textJson, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Marshal", err)
					return err
				}
				fmt.Println(string(textJson))
				return nil

			case "yaml":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Unmarshal", err)
					return err
				}
				textYaml, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Marshal", err)
					return err
				}
				fmt.Println(string(textYaml))
				return nil

			default:
				fmt.Println("output style must set text/json/yaml")
				return fmt.Errorf("output style must set text/json/yaml")
			}
		})
	},
}

func printHostStatus(status api.HostStatus) {
	nodeName := "N/A"
	if status.NodeName != nil {
		nodeName = *status.NodeName
	}
	ipAddress := "N/A"
	if status.IpAddress != nil {
		ipAddress = *status.IpAddress
	}
	fmt.Printf("ホスト情報:\n")
	fmt.Printf("  ノード名:       %s\n", nodeName)
	fmt.Printf("  IPアドレス:     %s\n", ipAddress)

	if status.Capacity != nil {
		c := status.Capacity
		cpuCores := 0
		if c.CpuCores != nil {
			cpuCores = *c.CpuCores
		}
		memoryMB := 0
		if c.MemoryMB != nil {
			memoryMB = *c.MemoryMB
		}
		diskCount := 0
		if c.DiskCount != nil {
			diskCount = *c.DiskCount
		}
		diskCap := 0
		if c.DiskCapacityGB != nil {
			diskCap = *c.DiskCapacityGB
		}
		fmt.Printf("\n資源搭載量:\n")
		fmt.Printf("  vCPU数:         %d\n", cpuCores)
		fmt.Printf("  メモリ搭載量:   %d MB\n", memoryMB)
		fmt.Printf("  ディスク本数:   %d\n", diskCount)
		fmt.Printf("  ディスク容量:   %d GB\n", diskCap)
		if c.NetworkInterfaces != nil && len(*c.NetworkInterfaces) > 0 {
			fmt.Printf("  ネットワークIF: %v\n", *c.NetworkInterfaces)
		}
	}

	if status.Allocation != nil {
		a := status.Allocation
		totalVMs := 0
		if a.TotalVMs != nil {
			totalVMs = *a.TotalVMs
		}
		runningVMs := 0
		if a.RunningVMs != nil {
			runningVMs = *a.RunningVMs
		}
		stoppedVMs := 0
		if a.StoppedVMs != nil {
			stoppedVMs = *a.StoppedVMs
		}
		allocCPU := 0
		if a.AllocatedCpuCores != nil {
			allocCPU = *a.AllocatedCpuCores
		}
		allocMem := 0
		if a.AllocatedMemoryMB != nil {
			allocMem = *a.AllocatedMemoryMB
		}
		vnets := 0
		if a.VirtualNetworks != nil {
			vnets = *a.VirtualNetworks
		}
		fmt.Printf("\n割当数:\n")
		fmt.Printf("  VM数（合計）:       %d\n", totalVMs)
		fmt.Printf("  VM数（稼働中）:     %d\n", runningVMs)
		fmt.Printf("  VM数（停止中）:     %d\n", stoppedVMs)
		fmt.Printf("  vCPU割当数:         %d vCPU（稼働中のみ）\n", allocCPU)
		fmt.Printf("  メモリ割当量:       %d MB（稼働中のみ）\n", allocMem)
		fmt.Printf("  仮想ネットワーク数: %d\n", vnets)
	}

	if status.LastUpdated != nil {
		fmt.Printf("\n最終更新: %s\n", status.LastUpdated.Format("2006-01-02 15:04:05"))
	}
}

func init() {
	marmotCmd.AddCommand(marmotStatusCmd)
}
