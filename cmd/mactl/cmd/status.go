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

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "管理下の仮想マシンをリストします。",
	Long: `管理下の仮想マシンをリストします。カレントディレクトリに cluster-config.yaml が存在しなければ動作しません。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		m, err := getClientConfig()
		if err != nil {
			slog.Error("faild reading mactl config file", "err", err.Error())
			return
		}

		clusterConfig, err := config.ReadYamlClusterConfig(clusterConfigFilename)
		if err != nil {
			slog.Error("failed reading cluster-config file", "err", err.Error())
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
		fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
			"CLUSTER", "VM-NAME", "H-Visr", "STAT", "VKEY", "VCPU", "RAM", "PubIP", "PriIP")
		fmt.Printf("%-20s", "DATA STORAGE")
		fmt.Printf("\n")

		for dec.More() {
			// クラスタ名と仮想マシンが一致したものだけリスト
			var vm api.VirtualMachine
			err := dec.Decode(&vm)
			if err != nil {
				slog.Error("list vms in the cluster", "err", err)
			}
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
				fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
					*vm.ClusterName, vm.Name, vm.HvNode, StateDsp[*vm.Status],
					*vm.Key, *vm.Cpu, *vm.Memory, *vm.PrivateIp, *vm.PublicIp)
				for _, dv := range *vm.Storage {
					fmt.Printf("%-4d", *dv.Size)
				}
				fmt.Printf("\n")
			}
		}
		dec.Token()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.PersistentFlags().StringVarP(&clusterConfigFilename, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
