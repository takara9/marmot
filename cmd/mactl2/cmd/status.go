/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "管理下の仮想マシンをリストします。",
	Long:  `管理下の仮想マシンをリストします。カレントディレクトリに cluster-config.yaml が存在しなければ動作しません。
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
		ListVm(cnf, ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
