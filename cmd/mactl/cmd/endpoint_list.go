package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var epListCmd = &cobra.Command{
	Use:   "list",
	Short: "エンドポイント一覧を表示",
	Long:  `登録されているMarmotサーバーのエンドポイント一覧を表示します。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.ReadMarmotConfig(config.MarmotConfigPath())
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("エンドポイントが登録されていません。")
				fmt.Println("'mactl ep add <URL>' でエンドポイントを追加してください。")
				return nil
			}
			return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
		}

		if len(cfg.Endpoints) == 0 {
			fmt.Println("エンドポイントが登録されていません。")
			fmt.Println("'mactl ep add <URL>' でエンドポイントを追加してください。")
			return nil
		}

		fmt.Printf("  %-4s  %-8s  %s\n", "No", "Status", "URL")
		fmt.Printf("  %-4s  %-8s  %s\n", "----", "--------", "-----------------------------")
		for i, ep := range cfg.Endpoints {
			status := "        "
			if i == cfg.Current {
				status = "* active"
			}
			fmt.Printf("  %-4d  %-8s  %s\n", i+1, status, ep)
		}
		return nil
	},
}

func init() {
	epCmd.AddCommand(epListCmd)
}
