package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var userLockCmd = &cobra.Command{
	Use:   "lock USER-ID",
	Short: "Lock a user account (admin only)",
	Long:  `Lock a user account to prevent login.`,
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

		if err := m.LockUserById(userID); err != nil {
			return fmt.Errorf("failed to lock user: %w", err)
		}

		fmt.Printf("User '%s' locked successfully\n", userID)
		return nil
	},
}

var userUnlockCmd = &cobra.Command{
	Use:   "unlock USER-ID",
	Short: "Unlock a user account (admin only)",
	Long:  `Unlock a previously locked user account to allow login.`,
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

		if err := m.UnlockUserById(userID); err != nil {
			return fmt.Errorf("failed to unlock user: %w", err)
		}

		fmt.Printf("User '%s' unlocked successfully\n", userID)
		return nil
	},
}

func init() {
	userCmd.AddCommand(userLockCmd)
	userCmd.AddCommand(userUnlockCmd)
}
