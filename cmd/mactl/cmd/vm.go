package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/types"
)

var ClusterName string
var VirtualMachineName string

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "仮想マシンの詳細を表示",
	Long:  `仮想マシンの内部情報を含めた詳細な情報を表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		m, err := getClientConfig()
		if err != nil {
			return
		}

		err = config.ReadConfig(ClusterConfig, &cnf)
		if err != nil {
			fmt.Println("Reading the config file err=", err)
			return
		}

		_, byteBody, _, err := m.ListVirtualMachines(nil)
		if err != nil {
			slog.Error("list vms", "err", err)
			return
		}
		StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}
		dec := json.NewDecoder(strings.NewReader(string(byteBody)))

		dec.Token()
		match := false
		var vm types.VirtualMachine

		for dec.More() {
			// クラスタ名と仮想マシンが一致したものだけリスト
			err := dec.Decode(&vm)
			if err != nil {
				slog.Error("listing vm status on cluster", "err", err)
			}

			// フィルター処理
			if ClusterName == vm.ClusterName {
				if VirtualMachineName == vm.Name {
					match = true
					break
				}
			}

		}
		// 表示
		if match {
			fmt.Printf("\n*** Virtual Machine Detail Info ***\n")
			fmt.Printf("\n")
			fmt.Printf("Cluster Name : %s \n", vm.ClusterName)
			fmt.Printf("Virtual Machine Name : %s \n", vm.Name)
			fmt.Printf("UUID : %s\n", vm.Uuid)
			fmt.Printf("Hypervisor : %s\n", vm.HvNode)
			fmt.Printf("Key: %s \n", vm.Key)
			fmt.Printf("Create Time: %s\n", vm.Ctime)
			fmt.Printf("Start  Time: %s\n", vm.Stime)
			fmt.Printf("Status : %s \n", StateDsp[vm.Status])
			fmt.Printf("CPU : %d \n", vm.Cpu)
			fmt.Printf("Memory(MB) : %d\n", vm.Memory)
			fmt.Printf("Private IP addr : %s\n", vm.PrivateIp)
			fmt.Printf("Public  IP addr : %s\n", vm.PublicIp)
			fmt.Printf("\n")
			fmt.Printf("OS Storage\n")
			fmt.Printf("    Volume Group Name : %s\n", vm.OsVg)
			fmt.Printf("    Logical Volume Name : %s\n", vm.OsLv)
			fmt.Printf("    OS Variant : %s\n", vm.OsVariant)
			fmt.Printf("\n")
			fmt.Printf("Data Storage\n")
			for _, v := range vm.Storage {
				fmt.Printf("    Storage Name : %s\n", v.Name)
				fmt.Printf("    Size(GB) : %d\n", v.Size)
				fmt.Printf("    Volume Group Name : %s\n", v.Vg)
				fmt.Printf("    Logical Volume Name : %s\n", v.Lv)
				fmt.Printf("    Path : %s\n", v.Path)
			}
			fmt.Printf("\n")
			fmt.Printf("Comment: %s \n", vm.Comment)
			fmt.Printf("Ansible Playbook: %s \n", vm.Playbook)
			fmt.Printf("\n")
		}
		dec.Token()
	},
}

func init() {
	rootCmd.AddCommand(vmCmd)
	vmCmd.Flags().StringVarP(&ClusterName, "cluster", "c", "", "Cluster name (required)")
	vmCmd.Flags().StringVarP(&VirtualMachineName, "vmname", "v", "", "Virtual machine name (required)")
	vmCmd.MarkFlagRequired("cluster")
	vmCmd.MarkFlagRequired("vmname")
}
