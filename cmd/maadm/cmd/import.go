/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// importCmd represents the migrate command
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "エクスポートされたデータをmarmotdのデータベースに読み込む",
	Long: `エクスポートされた marmotd の管理データをインポートします。バージョンアップ前に本コマンドでデータをバックアップして、バージョンを更新した後にインポートすることでデータの移行が実施できます。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("migrate called")
	},
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// migrateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// migrateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	importCmd.PersistentFlags().StringVarP(&etcdUrl, "url", "e", "http://localhost:2379", "URL address of the etcd server")
}
