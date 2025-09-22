package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "管理下の仮想マシンをシャットダウンして、CPUとメモリ資源を開放します。しかし、仮想マシンの定義は存続します。",
	Long: `管理下の仮想マシンをシャットダウンして、CPUとメモリ資源を開放しますが、仮想マシンの定義は存続し、startコマンドで再開できます。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		err := cf.ReadConfig(ClusterConfig, &cnf)
		if err != nil {
			fmt.Printf("Reading the config file", "err", err)
			return
		}

		if len(apiEndpoint) > 0 {
			ApiUrl = apiEndpoint
		}
		ReqRest(cnf, "stopCluster", ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.PersistentFlags().StringVarP(&ClusterConfig, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
