package cmd

import (
	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users and API keys",
	Long:  `User management commands for administrators and personal API key operations.`,
}

func init() {
	rootCmd.AddCommand(userCmd)
}
