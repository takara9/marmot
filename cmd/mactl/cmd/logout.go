package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout [USER-ID]",
	Short: "Logout from marmotd",
	Long:  `Clear the access token, logout from the current session, and remove user session API keys.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			return err
		}

		if err := loadTokenForEndpoint(m); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Failed to load token:", err)
		}

		targetUserID := ""
		if len(args) == 1 {
			targetUserID = strings.TrimSpace(args[0])
		}
		if targetUserID == "" {
			me, meErr := m.AuthMe()
			if meErr != nil {
				fmt.Fprintln(os.Stderr, "Warning: Failed to resolve current user:", meErr)
			} else if me != nil {
				targetUserID = strings.TrimSpace(me.UserId)
			}
		}

		deletedCount := 0
		if targetUserID != "" {
			keys, listErr := m.ListUserApiKeys(targetUserID)
			if listErr != nil {
				return fmt.Errorf("failed to list session API keys for user '%s': %w", targetUserID, listErr)
			}

			// Extract current session token prefix from the local access token
			currentTokenPrefix := ""
			if token := strings.TrimSpace(m.AccessToken); len(token) >= 8 {
				currentTokenPrefix = token[:8]
			}

			// Delete only the API key matching the current session token prefix
			for _, key := range keys {
				if currentTokenPrefix == "" {
					// If we can't determine current session, don't delete anything
					break
				}
				if key.Spec.TokenPrefix == nil || strings.TrimSpace(*key.Spec.TokenPrefix) != currentTokenPrefix {
					// Skip non-matching keys (other sessions)
					continue
				}
				// Delete only this key (current session)
				id := strings.TrimSpace(key.Metadata.Id)
				if id == "" {
					id = strings.TrimSpace(key.Metadata.Name)
				}
				if id == "" {
					continue
				}
				if err := m.DeleteUserApiKey(targetUserID, id); err != nil {
					return fmt.Errorf("failed to delete current session API key '%s' for user '%s': %w", id, targetUserID, err)
				}
				deletedCount++
			}
		}

		if err := m.AuthLogout(); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: Failed to logout from server:", err)
		}

		if err := clearAccessToken(); err != nil {
			return fmt.Errorf("failed to clear token: %w", err)
		}

		if targetUserID != "" {
			fmt.Printf("Successfully logged out (user=%s, removedSessions=%d)\n", targetUserID, deletedCount)
		} else {
			fmt.Println("Successfully logged out")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
