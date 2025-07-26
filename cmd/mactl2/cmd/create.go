/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create virtual machine",
	Long:  `Create virtual machine and run under marmot control hypervisors.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := cf.ReadConfig("cluster-config.yaml", &cnf)
		if err != nil {
			fmt.Printf("Reading the config file", "err", err)
			return
		}
		if len(apiEndpoint) > 0 {
			ApiUrl = apiEndpoint
		}
		ReqRest(cnf, "createCluster", ApiUrl)
		//if *auto {
		//	apply_playbook(cnf)
		//}
	},
}

func init() {
	rootCmd.AddCommand(createCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
