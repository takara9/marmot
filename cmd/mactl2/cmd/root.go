package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmot"
)

type Config struct {
	ApiServerUrl string `yaml:"api_server"`
}

var apiConfig string
var apiEndpoint string
var cliConfig Config
var ApiUrl string
var cnf config.MarmotConfig
var cfgFile string
var ClusterConfig string
var marmotClient *marmot.MarmotEndpoint

// BODYのJSONエラーメッセージ処理用
type msg struct {
	Msg string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mactl2",
	Short: "Marmot コントロールコマンド",
	Long:  `mactl は、ローカルPC上で QEMU, KVM、LVM, OpenSwitchを使用して実験や学習用の仮想マシン環境を提供します。`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	//var err error
	rootCmd.PersistentFlags().StringVar(&apiConfig, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
	rootCmd.Flags().BoolP("toggle", "t", false, "ヘルプメッセージの表示を切り替えます")

	/*
		// ここで失敗すると、ヘルプ画面が表示されないので、この処理は他に移動するべき
		fmt.Println("--- API Endpoint=", apiEndpoint)
		fmt.Println("--- API Endpoint=", apiEndpoint)
		fmt.Println("--- API Endpoint=", apiEndpoint)

		config.ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &DefaultConfig)


		if len(DefaultConfig.ApiServerUrl) > 0 {
			ApiUrl += DefaultConfig.ApiServerUrl
		}
		fmt.Println("DefaultConfig.ApiServerUrl=", DefaultConfig.ApiServerUrl)

		fmt.Println("ApiUrl=", ApiUrl)
		u, err := url.Parse(ApiUrl)
		if err != nil {
			os.Exit(1)
		}

		marmotClient, err = marmot.NewMarmotdEp(
			u.Scheme,
			u.Host,
			"/api/v1",
			60,
		)
		if err != nil {
			os.Exit(1)
		}
		// --------------------------------------------------------

		//rootCmd.PersistentFlags().StringVar(&apiEndpoint, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
		//rootCmd.Flags().BoolP("toggle", "t", false, "ヘルプメッセージの表示を切り替えます")
	*/
}
