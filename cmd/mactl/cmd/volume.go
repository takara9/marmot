package cmd

import "github.com/spf13/cobra"

var (
	volumeType    string // lv, qcow2, erc
	volumeKind    string // os, data
	templateImage bool   // ボリュームPLUMRテンプレート
)

var volumeCmd = &cobra.Command{
	Use:   "volume",
	Short: "Volume management commands",
}

func init() {
	rootCmd.AddCommand(volumeCmd)
}
