package cmd

import "github.com/spf13/cobra"

var (
	serverName    string // ボリューム名
	serverType    string // lv, qcow2, erc
	serverKind    string // os, data
	serverSize    int    // サイズ（GB単位）
	serverId      string // ボリュームID
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server management commands",
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

