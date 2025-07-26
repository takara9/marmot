/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	cf "github.com/takara9/marmot/pkg/config"
	//db "github.com/takara9/marmot/pkg/db"
)

type defaultConfig struct {
	ApiServerUrl string `yaml:"api_server"`
}

var DefaultConfig defaultConfig
var ApiUrl string
var cnf cf.MarmotConfig
// BODYのJSONエラーメッセージ処理用
type msg struct {
	Msg string
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mactl",
	Short: "Marmot control command",
	Long:  `mactl command use to control Marmot that is Virtual machine controller for experimental or learning`,
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
	cf.ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &DefaultConfig)

	fmt.Println("conf=", DefaultConfig)

	// パラメーター > コンフィグ
	if len(DefaultConfig.ApiServerUrl) > 0 {
		ApiUrl += DefaultConfig.ApiServerUrl
	}
	ccf := "cluster-config.yaml"
	err := cf.ReadConfig(ccf, &cnf)
	if err != nil {
		fmt.Printf("Reading the config file", "err", err)
		os.Exit(1)
	}

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.mactl.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
