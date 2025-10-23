package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンの表示",
	Long:  `marmot クライアントのバージョンを表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		slog.Debug("===", "versionCmd is called", "===")

		m, err := getClientConfig()
		if err != nil {
			slog.Error("version","err",err)
			return
		}

		JsonVersion, err := m.GetVersion()
		if err != nil {
			slog.Error("version","err",err)
			return
		}

		fmt.Println(string(JsonVersion.Version))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
