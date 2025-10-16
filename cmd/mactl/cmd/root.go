package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/client"
)

type Config struct {
	ApiServerUrl string `yaml:"api_server"`
}

var apiConfig string
var apiEndpoint string
var cliConfig Config
var ApiUrl string
var cnf config.MarmotConfig
var cfgFile string
var ClusterConfig string
var marmotClient *client.MarmotEndpoint

// BODYのJSONエラーメッセージ処理用
type msg struct {
	Msg string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mactl",
	Short: "Marmot コントロールコマンド",
	Long:  `mactl は、ローカルPC上で QEMU, KVM、LVM, OpenSwitchを使用して実験や学習用の仮想マシン環境を提供します。`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiConfig, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
	rootCmd.Flags().BoolP("toggle", "t", false, "ヘルプメッセージの表示を切り替えます")
}
