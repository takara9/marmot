package cmd

import (
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
)

var endpointComment string

var epAddCmd = &cobra.Command{
	Use:   "add <URL>",
	Short: "エンドポイントを追加する",
	Long: `MarmotサーバーのエンドポイントURLを追加します。
例: mactl ep add http://hv1:8750 -c "テスト環境"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]

		// URLの形式を検証
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("無効なURL形式です: %s", rawURL)
		}

		cfgPath := config.MarmotConfigPath()
		var cfg *config.MarmotConfig

		cfg, err = config.ReadMarmotConfig(cfgPath)
		if err != nil {
			if os.IsNotExist(err) {
				// 初回作成
				cfg = &config.MarmotConfig{
					Current:          0,
					Endpoints:        []string{},
					EndpointComments: []string{},
				}
			} else {
				return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
			}
		}
		cfg.NormalizeEndpointComments()

		// 重複チェック
		for _, ep := range cfg.Endpoints {
			if ep == rawURL {
				return fmt.Errorf("既に登録済みのエンドポイントです: %s", rawURL)
			}
		}

		cfg.Endpoints = append(cfg.Endpoints, rawURL)
		cfg.EndpointComments = append(cfg.EndpointComments, endpointComment)
		if err := config.WriteMarmotConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("設定ファイルの書き込みに失敗しました: %w", err)
		}

		num := len(cfg.Endpoints)
		fmt.Printf("エンドポイントを追加しました: [%d] %s\n", num, rawURL)
		if endpointComment != "" {
			fmt.Printf("  コメント: %s\n", endpointComment)
		}
		fmt.Printf("'mactl ep set %d' でアクティブに切り替えることができます。\n", num)
		return nil
	},
}

var epDeleteCmd = &cobra.Command{
	Use:   "delete <番号>",
	Short: "エンドポイントを削除する",
	Long: `指定した番号のエンドポイントを削除します。
番号は 'mactl ep list' で表示される No の値を指定してください。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var num int
		if _, err := fmt.Sscanf(args[0], "%d", &num); err != nil {
			return fmt.Errorf("番号の形式が不正です: %s", args[0])
		}

		cfgPath := config.MarmotConfigPath()
		cfg, err := config.ReadMarmotConfig(cfgPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("設定ファイルが見つかりません")
			}
			return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
		}

		idx := num - 1
		if idx < 0 || idx >= len(cfg.Endpoints) {
			return fmt.Errorf("番号 %d は範囲外です (1〜%d)", num, len(cfg.Endpoints))
		}
		cfg.NormalizeEndpointComments()

		deleted := cfg.Endpoints[idx]
		deletedComment := cfg.EndpointComment(idx)
		cfg.Endpoints = append(cfg.Endpoints[:idx], cfg.Endpoints[idx+1:]...)
		cfg.EndpointComments = append(cfg.EndpointComments[:idx], cfg.EndpointComments[idx+1:]...)

		// アクティブインデックスの調整
		if cfg.Current >= len(cfg.Endpoints) && cfg.Current > 0 {
			cfg.Current = len(cfg.Endpoints) - 1
		} else if cfg.Current == idx && len(cfg.Endpoints) > 0 {
			cfg.Current = 0
		}

		if err := config.WriteMarmotConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("設定ファイルの書き込みに失敗しました: %w", err)
		}

		fmt.Printf("エンドポイントを削除しました: %s\n", deleted)
		if deletedComment != "" {
			fmt.Printf("  コメント: %s\n", deletedComment)
		}
		return nil
	},
}

func init() {
	epAddCmd.Flags().StringVarP(&endpointComment, "comment", "c", "", "エンドポイントのコメント")
	epCmd.AddCommand(epAddCmd)
	epCmd.AddCommand(epDeleteCmd)
}
