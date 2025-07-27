/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var ClusterName string
var VirtualMachineName string

// vmCmd represents the vm command
var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "仮想マシンの詳細を表示",
	Long:  `仮想マシンの内部情報を含めた詳細な情報を表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		DetailVm(cnf, ApiUrl, ClusterName, VirtualMachineName)
	},
}

func init() {
	rootCmd.AddCommand(vmCmd)
	vmCmd.Flags().StringVarP(&ClusterName, "cluster", "c", "", "Cluster name (required)")
	vmCmd.Flags().StringVarP(&VirtualMachineName, "vmname", "v", "", "Virtual machine name (required)")
	vmCmd.MarkFlagRequired("cluster")
	vmCmd.MarkFlagRequired("vmname")
}
