/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// globalStatusCmd represents the globalStatus command
var globalStatusCmd = &cobra.Command{
	Use:   "globalStatus",
	Short: "管理下のハイパーバイザーと仮想マシンをリストします。",
	Long:  `管理下のハイパーバイザーと仮想マシンをリストします。デフォルトでホームディレクトリの.config_marmotを使用して、ハイパーバイザーに接続します。`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(apiEndpoint) > 0 {
			ApiUrl = apiEndpoint
		}
		fmt.Printf("\n               *** SYSTEM STATUS ***\n")
		ListHv(ApiUrl)
		fmt.Printf("\n")
		GlobalListVm(ApiUrl)
		fmt.Printf("\n")
	},
}

func init() {
	rootCmd.AddCommand(globalStatusCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// globalStatusCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// globalStatusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
