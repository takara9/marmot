package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from marmotd",
	Long:  `Clear the access token and logout from the current session.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			return err
		}

		if err := loadTokenForEndpoint(m); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Failed to load token:", err)
		}

		if err := m.AuthLogout(); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Failed to logout from server:", err)
		}

		if err := clearAccessToken(); err != nil {
			return fmt.Errorf("failed to clear token: %w", err)
		}

		fmt.Println("Successfully logged out")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
