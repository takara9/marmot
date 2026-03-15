package cmd

import "github.com/spf13/cobra"

var (
	imageName    string // ボリューム名
//	volumeType    string // lv, qcow2, erc
//	volumeKind    string // os, data
//	volumeSize    int    // サイズ（GB単位）
//	volumeId      string // ボリュームID
//	templateImage bool   // ボリュームPLUMRテンプレート
//	osName        string // OS名
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Image management commands",
}

func init() {
	rootCmd.AddCommand(imageCmd)
}
