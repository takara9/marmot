package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "仮想マシンの生成と起動",
	Long: `管理下のハイパーバイザーの一つに仮想マシンをスケジュールして生成と起動を実施します。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		m, err := getClientConfig()
		if err != nil {
			return
		}

		clusterConfig, err := config.ReadYamlClusterConfig(ClusterConfig)
		if err != nil {
			fmt.Println("Reading the config file err=", err)
			return
		}

		//
		PrintMarmotConfig(*clusterConfig)
		//

		_, _, _, err = m.CreateCluster(*clusterConfig)
		if err != nil {
			fmt.Println("failed to create VM cluster: ", err)
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().StringVarP(&ClusterConfig, "config", "c", "cluster-config.yaml", "")
}
