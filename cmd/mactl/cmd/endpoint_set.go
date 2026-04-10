package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var epSetCmd = &cobra.Command{
	Use:   "set <番号>",
	Short: "アクティブなエンドポイントを切り替える",
	Long: `指定した番号のエンドポイントをアクティブにします。
番号は 'mactl ep list' で表示される No の値を指定してください。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		num, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("番号の形式が不正です: %s", args[0])
		}

		cfgPath := config.MarmotConfigPath()
		cfg, err := config.ReadMarmotConfig(cfgPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("設定ファイルが見つかりません。'mactl ep add <URL>' でエンドポイントを追加してください")
			}
			return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
		}

		// 入力は1始まり、内部インデックスは0始まり
		idx := num - 1
		if idx < 0 || idx >= len(cfg.Endpoints) {
			return fmt.Errorf("番号 %d は範囲外です (1〜%d)", num, len(cfg.Endpoints))
		}

		cfg.Current = idx
		if err := config.WriteMarmotConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("設定ファイルの書き込みに失敗しました: %w", err)
		}

		fmt.Printf("エンドポイントを切り替えました: [%d] %s\n", num, cfg.Endpoints[idx])
		return nil
	},
}

func init() {
	epCmd.AddCommand(epSetCmd)
}
