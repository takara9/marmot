package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

// createCmd represents the create command
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

		err = config.ReadConfig(ClusterConfig, &cnf)
		if err != nil {
			fmt.Println("Reading the config file err=", err)
			return
		}
		_, _, _, err = m.CreateCluster(cnf)
		if err != nil {
			fmt.Println("failed to create VM cluster: ", err)
			return
		}
		//ReqRest(cnf, "createCluster", ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().StringVarP(&ClusterConfig, "config", "c", "cluster-config.yaml", "")
}
