/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	cf "github.com/takara9/marmot/pkg/config"
	//db "github.com/takara9/marmot/pkg/db"
)

type defaultConfig struct {
	ApiServerUrl string `yaml:"api_server"`
}

var DefaultConfig defaultConfig
var ApiUrl string
var cnf cf.MarmotConfig
var cfgFile string
var apiEndpoint string
var ClusterConfig string


// BODYのJSONエラーメッセージ処理用
type msg struct {
	Msg string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mactl2",
	Short: "Marmot コントロールコマンド",
	Long:  `mactl は、ローカルPC上で QEMU, KVM、LVM, OpenSwitchを使用して実験や学習用の仮想マシン環境を提供します。`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// パラメーター > コンフィグ
	cf.ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &DefaultConfig)
	if len(DefaultConfig.ApiServerUrl) > 0 {
		ApiUrl += DefaultConfig.ApiServerUrl
	}

	// marmot-client の初期化
	// NewMarmotdEp()

	rootCmd.PersistentFlags().StringVar(&apiEndpoint, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
	rootCmd.Flags().BoolP("toggle", "t", false, "ヘルプメッセージの表示を切り替えます")
}
