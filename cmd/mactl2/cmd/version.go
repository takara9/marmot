package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンの表示",
	Long:  `marmot クライアントのバージョンを表示します。`,
	Run: func(cmd *cobra.Command, args []string) {

		m, err := getClientConfig()
		if err != nil {
			fmt.Println("err=", err)
			return
		}
		JsonVersion, err := m.GetVersion()
		if err != nil {
			fmt.Println("err=", err)
			return
		}
		fmt.Println(string(JsonVersion.Version))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
