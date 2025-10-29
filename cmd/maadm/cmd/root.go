/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	//"github.com/takara9/marmot/pkg/config"
)

var apiConfigFilename string
var etcdUrl string

//var mactlConfig config.ClientConfig
//var mactlConfig config.ClientConfig

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "maadm",
	Short: "Marmot システム管理コマンド",
	Long:  `初期データのインストール、バージョンアップ時のデータ移行など、marmot のシステム管理の機能を提供します。`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		slog.Error("エラー終了", "err", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiConfigFilename, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
