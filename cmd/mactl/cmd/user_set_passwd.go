package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"golang.org/x/term"
)

var userSetPasswdCmd = &cobra.Command{
	Use:   "set-passwd USER-ID",
	Short: "Set a user's password (admin only)",
	Long:  `Set or reset the password for a specified user.`,
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

		passwdStr, _ := cmd.Flags().GetString("passwd")

		if strings.TrimSpace(passwdStr) == "" {
			fmt.Print("New password: ")
			pwd, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			fmt.Println()
			passwdStr = string(pwd)
		}

		if strings.TrimSpace(passwdStr) == "" {
			return fmt.Errorf("password cannot be empty")
		}

		pwdReq := api.PasswordChangeRequest{
			NewPassword: passwdStr,
		}

		if err := m.ChangeUserPassword(userID, pwdReq); err != nil {
			return fmt.Errorf("failed to set password: %w", err)
		}

		fmt.Printf("Password for user '%s' set successfully\n", userID)
		return nil
	},
}

func init() {
	userSetPasswdCmd.Flags().String("passwd", "", "Password (if not provided, will prompt)")
	userCmd.AddCommand(userSetPasswdCmd)
}
