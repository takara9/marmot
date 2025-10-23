package cmd

import (
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "管理下の仮想マシンをシャットダウンして、CPUとメモリ資源を開放します。しかし、仮想マシンの定義は存続します。",
	Long: `管理下の仮想マシンをシャットダウンして、CPUとメモリ資源を開放しますが、仮想マシンの定義は存続し、startコマンドで再開できます。
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
		_, _, _, err = m.StopCluster(*clusterConfig)
		if err != nil {
			slog.Error("failed to stop virtual machines", "err", err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.PersistentFlags().StringVarP(&clusterConfigFilename, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
