package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"marmot.io/config"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "仮想マシンをシャットダウンして定義を削除します",
	Long: `管理下のハイパーバイザー上の仮想マシンのシャットダウンと定義の削除を実施します。
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

		_, _, _, err = m.DestroyCluster(cnf)
		if err != nil {
			fmt.Println("failed to destroy VM cluster: ", err)
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.PersistentFlags().StringVarP(&ClusterConfig, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
