package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var marmotClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Show marmot cluster members",
	Long:  `Show all marmot cluster member statuses, including node name, host id, and VM allocation counts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
			m, err := getClientConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
				os.Exit(1)
			}

			byteBody, _, err := m.GetMarmotCluster()
			if err != nil {
				fmt.Fprintln(os.Stderr, "エラー応答が返されました。", err)
				return nil
			}

			var statuses []api.HostStatus
			switch outputStyle {
			case "text":
				if err := json.Unmarshal(byteBody, &statuses); err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Unmarshal", err)
					return err
				}
				printHostCluster(statuses)
				return nil

			case "json":
				if err := json.Unmarshal(byteBody, &statuses); err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Unmarshal", err)
					return err
				}
				textJSON, err := json.MarshalIndent(statuses, "", "  ")
				if err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Marshal", err)
					return err
				}
				fmt.Println(string(textJSON))
				return nil

			case "yaml":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Unmarshal", err)
					return err
				}
				textYAML, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Failed to Marshal", err)
					return err
				}
				fmt.Println(string(textYAML))
				return nil

			default:
				fmt.Println("output style must set text/json/yaml")
				return fmt.Errorf("output style must set text/json/yaml")
			}
		})
	},
}

func printHostCluster(statuses []api.HostStatus) {
	if len(statuses) == 0 {
		fmt.Println("No cluster members found.")
		return
	}

	sort.Slice(statuses, func(i, j int) bool {
		return stringVal(statuses[i].NodeName) < stringVal(statuses[j].NodeName)
	})

	fmt.Printf("%-16s %-10s %-15s %7s %10s %6s %7s %7s %8s %8s %8s %s\n",
		"NODE", "HOSTID", "IP", "CAP_CPU", "CAP_MEM(MB)", "TOTAL", "RUNNING", "STOPPED", "VCPU", "MEM(MB)", "VNETS", "UPDATED")
	for _, s := range statuses {
		a := s.Allocation
		c := s.Capacity
		fmt.Printf("%-16s %-10s %-15s %7d %10d %6d %7d %7d %8d %8d %8d %s\n",
			stringVal(s.NodeName),
			stringVal(s.HostId),
			stringVal(s.IpAddress),
			capacityIntVal(c, func(x *api.HostCapacity) *int { return x.CpuCores }),
			capacityIntVal(c, func(x *api.HostCapacity) *int { return x.MemoryMB }),
			intVal(a, func(x *api.HostAllocation) *int { return x.TotalVMs }),
			intVal(a, func(x *api.HostAllocation) *int { return x.RunningVMs }),
			intVal(a, func(x *api.HostAllocation) *int { return x.StoppedVMs }),
			intVal(a, func(x *api.HostAllocation) *int { return x.AllocatedCpuCores }),
			intVal(a, func(x *api.HostAllocation) *int { return x.AllocatedMemoryMB }),
			intVal(a, func(x *api.HostAllocation) *int { return x.VirtualNetworks }),
			timeVal(s.LastUpdated),
		)
	}
}

func stringVal(v *string) string {
	if v == nil || *v == "" {
		return "N/A"
	}
	return *v
}

func intVal(a *api.HostAllocation, f func(*api.HostAllocation) *int) int {
	if a == nil {
		return 0
	}
	v := f(a)
	if v == nil {
		return 0
	}
	return *v
}

func capacityIntVal(c *api.HostCapacity, f func(*api.HostCapacity) *int) int {
	if c == nil {
		return 0
	}
	v := f(c)
	if v == nil {
		return 0
	}
	return *v
}

func timeVal(v *time.Time) string {
	if v == nil {
		return "N/A"
	}
	return v.Format("2006-01-02 15:04:05")
}

func init() {
	marmotCmd.AddCommand(marmotClusterCmd)
}
