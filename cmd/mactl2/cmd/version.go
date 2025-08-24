/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンの表示",
	Long:  `marmot クライアントのバージョンを表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("version: 0.8.6")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
