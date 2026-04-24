package cmd

import "github.com/spf13/cobra"

var marmotCmd = &cobra.Command{
	Use:   "marmot",
	Short: "Marmot host management commands",
}

func init() {
	rootCmd.AddCommand(marmotCmd)
}
