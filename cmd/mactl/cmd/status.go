package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
)

func nilToEmptyString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func nil64ToZero(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}
func nil32ToZero(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

func nilToTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.String()
}

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "管理下の仮想マシンをリストします。",
	Long: `管理下の仮想マシンをリストします。カレントディレクトリに cluster-config.yaml が存在しなければ動作しません。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		clusterConfig, err := config.ReadYamlClusterConfig(clusterConfigFilename)
		if err != nil {
			slog.Error("failed reading cluster-config file", "err", err.Error())
			return
		}

		slog.Debug("mactl ListVirtualMachines", "before", clusterConfig)
		_, byteBody, _, err := m.ListVirtualMachines(nil)
		if err != nil {
			slog.Error("list vms", "err", err)
			return
		}
		slog.Debug("mactl ListVirtualMachines", "body", string(byteBody))
		var vms []api.VirtualMachine
		err = json.Unmarshal(byteBody, &vms)
		if err != nil {
			slog.Error("Unmarshal", "err", err)
			return
		}

		fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
			"CLUSTER", "VM-NAME", "H-Visr", "STAT", "VKEY", "VCPU", "RAM", "PubIP", "PriIP")
		fmt.Printf("%-20s", "DATA STORAGE")
		fmt.Printf("\n")

		StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}
		for _, vm := range vms {
			// フィルター処理
			match := false
			if *clusterConfig.ClusterName == *vm.ClusterName {
				for _, spec := range *clusterConfig.VmSpec {
					if *spec.Name == vm.Name {
						match = true
						break
					}
				}
			}

			// 表示
			if match {
				k := strings.Split(*vm.Key, "/")
				fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ", nilToEmptyString(vm.ClusterName), vm.Name, vm.HvNode, StateDsp[*vm.Status], k[len(k)-1], nil32ToZero(vm.Cpu), nil64ToZero(vm.Memory), nilToEmptyString(vm.PrivateIp), nilToEmptyString(vm.PublicIp))
				if vm.Storage != nil {
					for _, dv := range *vm.Storage {
						fmt.Printf("%-4d", nil64ToZero(dv.Size))
					}
				}
				fmt.Printf("\n")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.PersistentFlags().StringVarP(&clusterConfigFilename, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
