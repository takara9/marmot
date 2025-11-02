package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var apiConfigFilename string

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンの表示",
	Long:  `maadm クライアントのバージョンを表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		slog.Debug("===", "versionCmd is called", "===")

		m, err := config.GetClientConfig2(apiConfigFilename)
		if err != nil {
			slog.Error("version", "err", err)
			return
		}

		JsonVersion, err := m.GetVersion()
		if err != nil {
			slog.Error("version", "err", err)
			return
		}

		fmt.Println(string(JsonVersion.Version))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.PersistentFlags().StringVar(&apiConfigFilename, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
}
