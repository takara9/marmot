package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var apiConfigFilename string
var outputStyle string
var watchMode bool
var watchInterval int
var labelSelector string

//var m *client.MarmotEndpoint

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mactl",
	Short: "Marmot コントロールコマンド",
	Long:  `mactl は、ローカルPC上で QEMU, KVM、LVM, OpenSwitchを使用して実験や学習用の仮想マシン環境を提供します。`,
}

func Execute() {
	normalizedArgs := normalizeInvocationArgs(os.Args)
	if len(normalizedArgs) > 1 {
		rootCmd.SetArgs(normalizedArgs[1:])
	} else {
		rootCmd.SetArgs([]string{})
	}
	defer rootCmd.SetArgs(nil)

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

func normalizeInvocationArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	if strings.TrimSpace(filepath.Base(args[0])) != "mactl-ssh" {
		return args
	}

	if len(args) > 1 && strings.TrimSpace(args[1]) == "ssh" {
		return args
	}

	normalized := make([]string, 0, len(args)+1)
	normalized = append(normalized, args[0], "ssh")
	normalized = append(normalized, args[1:]...)
	return normalized
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiConfigFilename, "api", "", "API Endpoint URL (default is $HOME/.marmot)")
	rootCmd.Flags().BoolP("toggle", "t", false, "ヘルプメッセージの表示を切り替えます")
	rootCmd.PersistentFlags().StringVarP(&outputStyle, "output", "o", "text", "Text style output")
	rootCmd.PersistentFlags().BoolVarP(&watchMode, "watch", "w", false, "変化があった時に表示を更新する")
	rootCmd.PersistentFlags().IntVar(&watchInterval, "watch-interval", 2, "Watchモードの更新間隔（秒）")
}
