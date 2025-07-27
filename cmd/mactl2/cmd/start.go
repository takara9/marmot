/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "停止中の仮想マシンを開始します。",
	Long: `stop で停止された仮想マシンの活動を再開します。
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
		ReqRest(cnf, "startCluster", ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.PersistentFlags().StringVarP(&ClusterConfig, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
