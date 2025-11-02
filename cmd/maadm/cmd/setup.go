package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("setup called")
		fmt.Println("etcd url =", etcdUrl)
		fmt.Println("hvconfig =", hypervisorConfigFilename)
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
	// コンフィグファイル
	setupCmd.PersistentFlags().StringVarP(&hypervisorConfigFilename, "hvconfig", "v", "hypervisor-config.yaml", "Initial Hypervisor configfile (yaml)")
	// etcdのURLアドレス
	setupCmd.PersistentFlags().StringVarP(&etcdUrl, "url", "e", "http://localhost:2379", "URL address of the etcd server")
}
