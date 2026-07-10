package cmd

import "github.com/spf13/cobra"

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Network management commands",
}

func init() {
	rootCmd.AddCommand(networkCmd)
}
