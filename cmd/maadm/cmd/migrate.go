/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "etcdサーバーのマイグレーションを実行するコマンドです。",
	Long: `このコマンドはetcdサーバーのマイグレーション処理を実行します。
サーバーのURLを指定して、必要なデータの移行や更新を行うことができます。

例:
  maadm migrate --url http://localhost:2379
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("migrate called")
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// migrateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// migrateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	migrateCmd.PersistentFlags().StringVarP(&etcdUrl, "url", "e", "http://localhost:2379", "URL address of the etcd server")
}
