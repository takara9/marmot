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
	Long:  `stop で停止された仮想マシンの活動を再開します。
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
		ReqRest(cnf, "startCluster", ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// startCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// startCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
