/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "marmot のデータをJSON形式でエクスポートする",
	Long: `marmot の管理データをJSON形式でエクスポートします。バージョンアップ前に本コマンドでデータをバックアップして、バージョンを更新した後にインポートすることでデータの移行を実施できます。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("export called")
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// migrateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// migrateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	exportCmd.PersistentFlags().StringVarP(&etcdUrl, "url", "e", "http://localhost:2379", "URL address of the etcd server")
}
