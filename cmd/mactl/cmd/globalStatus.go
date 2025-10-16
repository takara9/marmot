package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/types"
)

var globalStatusCmd = &cobra.Command{
	Use:   "globalStatus",
	Short: "管理下のハイパーバイザーと仮想マシンをリストします。",
	Long:  `管理下のハイパーバイザーと仮想マシンをリストします。デフォルトでホームディレクトリの.config_marmotを使用して、ハイパーバイザーに接続します。`,
	Run: func(cmd *cobra.Command, args []string) {
		m, err := getClientConfig()
		if err != nil {
			return
		}

		fmt.Printf("\n               *** SYSTEM STATUS ***\n")
		_, byteBody, _, err := m.ListHypervisors(nil)
		if err != nil {
			slog.Error("list vms", "err", err)
			return
		}
		status := [3]string{"HLT", "ERR", "RUN"}
		dec := json.NewDecoder(strings.NewReader(string(byteBody)))
		dec.Token()
		fmt.Printf("%-10s %-3v %-15v %-8v  %-12v   %-12v", "HV-NAME", "ONL", "IPaddr", "VCPU", "RAM(MB)", "Storage(GB)")
		fmt.Printf("\n")
		for dec.More() {
			var hv types.Hypervisor
			err := dec.Decode(&hv)
			if err != nil {
				slog.Error("reading hypervisors status", "err", err)
			}
			fmt.Printf("%-10s %-3v %-15v %4d/%-4d %6d/%-6d  ",
				hv.Nodename, status[hv.Status], hv.IpAddr, hv.FreeCpu, hv.Cpu, hv.FreeMemory, hv.Memory)
			for _, v := range hv.StgPool {
				fmt.Printf("%v(%v): %5d/%-5d ", v.VolGroup, v.Type, v.FreeCap, v.VgCap)
			}
			fmt.Printf("\n")
		}
		dec.Token()

		fmt.Printf("\n")

		_, byteBody, _, err = m.ListVirtualMachines(nil)
		if err != nil {
			slog.Error("list vms", "err", err)
			return
		}

		StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}
		if err != nil {
			slog.Error("global-status", "err", err)
			return
		}

		dec = json.NewDecoder(strings.NewReader(string(byteBody)))
		dec.Token()
		fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
			"CLUSTER", "VM-NAME", "H-Visr", "STAT", "VKEY", "VCPU", "RAM", "PubIP", "PriIP")
		fmt.Printf("%-20s", "DATA STORAGE")
		fmt.Printf("\n")
		for dec.More() {
			var vm types.VirtualMachine
			err := dec.Decode(&vm)
			if err != nil {
				slog.Error("getting vm status", "err", err)
			}
			fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
				vm.ClusterName, vm.Name, vm.HvNode, StateDsp[vm.Status],
				vm.Key, vm.Cpu, vm.Memory, vm.PrivateIp, vm.PublicIp)
			for _, dv := range vm.Storage {
				fmt.Printf("%-4d", dv.Size)
			}
			fmt.Printf("\n")
		}
		dec.Token()
		fmt.Printf("\n")
	},
}

func init() {
	rootCmd.AddCommand(globalStatusCmd)
}
