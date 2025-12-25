package cmd

import "github.com/spf13/cobra"

var (
	volumeName    string // ボリューム名
	volumeType    string // lv, qcow2, erc
	volumeKind    string // os, data
	volumeSize    int    // サイズ（GB単位）
	volumeId      string // ボリュームID
	templateImage bool   // ボリュームPLUMRテンプレート
	osName        string // OS名
)

var volumeCmd = &cobra.Command{
	Use:   "volume",
	Short: "Volume management commands",
}

func init() {
	rootCmd.AddCommand(volumeCmd)
}
