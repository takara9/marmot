package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
)

var hypervisorConfigFilename string
var etcdUrl string

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "marmotdの初期データを直接書き込むツール",
	Long: `marmotdサーバーのセットアップ時にetcd に対して、初期データを直接書き込むツール
	そのため、このコマンドは、marmotdを起動するサーバーで実行する必要があります。
	実行に際して「ハイパーバイザーの初期データのYAMLファイル」と「etcdのURLアドレス」を
	与える必要があります。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("setup called")
		fmt.Println("etcd url =", etcdUrl)
		fmt.Println("hvconfig =", hypervisorConfigFilename)

		hvs, err := config.ReadHypervisorConfig(hypervisorConfigFilename)
		if err != nil {
			fmt.Println("Error:", err)
			return err
		}

		if err = setHypervisorConfig(*hvs, etcdUrl); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
	// コンフィグファイル
	setupCmd.PersistentFlags().StringVarP(&hypervisorConfigFilename, "hvconfig", "v", "hypervisor-config.yaml", "Initial Hypervisor configfile (yaml)")
	// etcdのURLアドレス
	setupCmd.PersistentFlags().StringVarP(&etcdUrl, "etcdurl", "e", "http://localhost:2379", "URL address of the etcd server")
}

func setHypervisorConfig(hvs config.Hypervisors_yaml, kvsurl string) error {

	d, err := db.NewDatabase(kvsurl)
	if err != nil {
		slog.Error("Failed to connect to etcd", "error", err)
		return err
	}

	// ハイパーバイザーの初期設定をDBへセット
	for _, hv := range hvs.Hvs {
		if err := d.SetHypervisors(hv); err != nil {
			slog.Error("Failed to set hypervisor config", "error", err)
			return err
		}
	}

	// OSイメージテンプレートの初期設定をDBへセット
	for _, hd := range hvs.Imgs {
		if err := d.SetImageTemplate(hd); err != nil {
			slog.Error("Failed to set image template", "error", err)
			return err
		}
	}

	// シーケンス番号の初期値をDBへセット
	for _, sq := range hvs.Seq {
		if err := d.CreateSeq(sq.Key, sq.Start, sq.Step); err != nil {
			slog.Error("Failed to create sequence", "error", err)
			return err
		}
	}

	return nil
}
