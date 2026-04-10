package cmd

import "github.com/spf13/cobra"

// epCmd はエンドポイント管理コマンドの親コマンド
var epCmd = &cobra.Command{
	Use:   "ep",
	Short: "エンドポイント管理コマンド",
	Long:  `複数のハイパーバイザー(Marmotサーバー)エンドポイントを管理します。`,
}

func init() {
	rootCmd.AddCommand(epCmd)
}
