/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status VMs",
	Long:  `Show status of virtual machines.`,
	Run: func(cmd *cobra.Command, args []string) {
		ListVm(cnf, ApiUrl)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
