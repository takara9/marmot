package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var userDeleteCmd = &cobra.Command{
	Use:   "delete USER-ID",
	Short: "Delete a user (admin only)",
	Long:  `Delete an existing user account.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			return err
		}

		if err := loadTokenForEndpoint(m); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Failed to load token:", err)
		}

		userID := strings.TrimSpace(args[0])
		if userID == "" {
			return fmt.Errorf("user ID cannot be empty")
		}

		if err := m.DeleteUserById(userID); err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}

		fmt.Printf("User '%s' deleted successfully\n", userID)
		return nil
	},
}

func init() {
	userCmd.AddCommand(userDeleteCmd)
}
