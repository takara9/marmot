package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "停止中の仮想マシンを開始します。",
	Long: `stop で停止された仮想マシンの活動を再開します。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
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
		_, _, _, err = m.StartCluster(cnf)
		if err != nil {
			fmt.Println("failed to create VM cluster: ", err)
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.PersistentFlags().StringVarP(&ClusterConfig, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
