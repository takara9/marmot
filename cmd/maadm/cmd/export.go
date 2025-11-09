/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "marmot のデータをJSON形式でエクスポートする",
	Long:  `marmot の管理データをJSON形式でエクスポートします。バージョンアップ前に本コマンドでデータをバックアップして、バージョンを更新した後にインポートすることでデータの移行を実施できます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := db.NewDatabase(etcdUrl)
		if err != nil {
			slog.Error("Failed to connect to etcd", "error", err)
			return err
		}

		// ハイパーバイザー
		var hvs []types.Hypervisor
		err = d.GetHypervisors(&hvs)
		if err != nil {
			slog.Error("Failed to get hypervisor infos", "error", err)
			return err
		}
		fmt.Print(hvs)

		// OSイメージテンプレート
		var osit []types.OsImageTemplate
		err = d.GetOsImgTempes(&osit)
		if err != nil {
			slog.Error("Failed to get os image templates", "error", err)
			return err
		}
		fmt.Print(osit)

		// シーケンス番号
		var seqs []types.VmSerial
		err = d.GetSeqs(&seqs)
		if err != nil {
			slog.Error("Failed to get serials", "error", err)
			return err
		}
		fmt.Print(seqs)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.PersistentFlags().StringVarP(&etcdUrl, "etcdurl", "e", "http://localhost:2379", "URL address of the etcd server")
}
