package cmd

import "github.com/spf13/cobra"

var (
	serverName    string // ボリューム名
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server management commands",
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

