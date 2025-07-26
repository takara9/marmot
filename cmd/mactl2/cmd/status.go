/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status VMs",
	Long:  `Show status of virtual machines.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := cf.ReadConfig("cluster-config.yaml", &cnf)
		if err != nil {
			fmt.Printf("Reading the config file", "err", err)
			return
		}
		if len(apiEndpoint) > 0 {
			ApiUrl = apiEndpoint
		}
		ListVm(cnf, ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
