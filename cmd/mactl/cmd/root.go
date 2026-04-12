package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var apiConfigFilename string
var mactlConfig config.ClientConfig
var clusterConfigFilename string
var outputStyle string
var watchMode bool
var watchInterval int

//var m *client.MarmotEndpoint

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mactl",
	Short: "Marmot コントロールコマンド",
	Long:  `mactl は、ローカルPC上で QEMU, KVM、LVM, OpenSwitchを使用して実験や学習用の仮想マシン環境を提供します。`,
}

func Execute() {
	//var err error
	//m, err = getClientConfig()
	//if err != nil {
	//	fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
	//	os.Exit(1)
	//}
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed error:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiConfigFilename, "api", "", "API Endpoint URL (default is $HOME/.marmot)")
	rootCmd.Flags().BoolP("toggle", "t", false, "ヘルプメッセージの表示を切り替えます")
	rootCmd.PersistentFlags().StringVarP(&outputStyle, "output", "o", "text", "Text style output")
	rootCmd.PersistentFlags().BoolVarP(&watchMode, "watch", "w", false, "変化があった時に表示を更新する")
	rootCmd.PersistentFlags().IntVar(&watchInterval, "watch-interval", 2, "Watchモードの更新間隔（秒）")
}
