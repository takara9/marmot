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

	importCmd.PersistentFlags().StringVarP(&etcdUrl, "url", "e", "http://localhost:2379", "URL address of the etcd server")

}
