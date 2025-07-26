/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
)

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "仮想マシンをシャットダウンして定義を削除します",
	Long: `管理下のハイパーバイザー上の仮想マシンのシャットダウンと定義の削除を実施します。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		err := cf.ReadConfig("cluster-config.yaml", &cnf)
		if err != nil {
			fmt.Printf("Reading the config file", "err", err)
			return
		}
		if len(apiEndpoint) > 0 {
			ApiUrl = apiEndpoint
		}
		ReqRest(cnf, "destroyCluster", ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// destroyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// destroyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
