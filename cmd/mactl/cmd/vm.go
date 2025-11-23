package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
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
			slog.Error("faild reading mactl config file", "err", err.Error())
			return
		}

		clusterConfig, err := config.ReadYamlClusterConfig(clusterConfigFilename)
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
		var vm api.VirtualMachine

		for dec.More() {
			// クラスタ名と仮想マシンが一致したものだけリスト
			err := dec.Decode(&vm)
			if err != nil {
				slog.Error("listing vm status on cluster", "err", err)
			}

			// フィルター処理
			if *clusterConfig.ClusterName == *vm.ClusterName {
				//if VirtualMachineName == vm.Name {
				match = true
				break
				//}
			}

		}
		// 表示
		if match {
			fmt.Printf("\n*** Virtual Machine Detail Info ***\n")
			fmt.Printf("\n")
			fmt.Printf("Cluster Name : %s \n", vm.ClusterName)
			fmt.Printf("Virtual Machine Name : %s \n", vm.Name)
			fmt.Printf("UUID : %s\n", nilToEmptyString(vm.Uuid))
			fmt.Printf("Hypervisor : %s\n", vm.HvNode)
			fmt.Printf("Hypervisor's IP : %s\n", nilToEmptyString(vm.HvIpAddr))
			fmt.Printf("Key: %s \n", nilToEmptyString(vm.Key))
			fmt.Printf("Create Time: %s\n", nilToTime(vm.CTime))
			fmt.Printf("Start  Time: %s\n", nilToTime(vm.STime))
			fmt.Printf("Status : %s \n", StateDsp[nil32ToZero(vm.Status)])
			fmt.Printf("CPU : %d \n", nil32ToZero(vm.Cpu))
			fmt.Printf("Memory(MB) : %d\n", nil64ToZero(vm.Memory))
			fmt.Printf("Private IP addr : %s\n", nilToEmptyString(vm.PrivateIp))
			fmt.Printf("Public  IP addr : %s\n", nilToEmptyString(vm.PublicIp))
			fmt.Printf("\n")
			fmt.Printf("OS Storage\n")
			fmt.Printf("    Volume Group Name : %s\n", nilToEmptyString(vm.OsVg))
			fmt.Printf("    Logical Volume Name : %s\n", nilToEmptyString(vm.OsLv))
			fmt.Printf("    OS Variant : %s\n", nilToEmptyString(vm.OsVariant))
			fmt.Printf("\n")
			fmt.Printf("Data Storage\n")
			if vm.Storage != nil {
				for _, v := range *vm.Storage {
					fmt.Printf("    Storage Name : %s\n", nilToEmptyString(v.Name))
					fmt.Printf("    Size(GB) : %d\n", nil64ToZero(v.Size))
					fmt.Printf("    Volume Group Name : %s\n", nilToEmptyString(v.Vg))
					fmt.Printf("    Logical Volume Name : %s\n", nilToEmptyString(v.Lv))
					fmt.Printf("    Path : %s\n", nilToEmptyString(v.Path))
				}
			}
			fmt.Printf("\n")
			fmt.Printf("Comment: %s \n", nilToEmptyString(vm.Comment))
			fmt.Printf("Ansible Playbook: %s \n", nilToEmptyString(vm.Playbook))
			fmt.Printf("\n")
		}
		dec.Token()
	},
}

func init() {
	rootCmd.AddCommand(vmCmd)
	vmCmd.Flags().StringVarP(&ClusterName, "cluster", "n", "", "Cluster name (required)")
	vmCmd.Flags().StringVarP(&VirtualMachineName, "vmname", "v", "", "Virtual machine name (required)")
	vmCmd.MarkFlagRequired("cluster")
	vmCmd.MarkFlagRequired("vmname")
}
