package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"golang.org/x/term"
)

var passwdCmd = &cobra.Command{
	Use:   "passwd",
	Short: "Change your password",
	Long:  `Change the password for the currently logged-in user.`,
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

		me, err := m.AuthMe()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		if me == nil || strings.TrimSpace(me.UserId) == "" {
			return fmt.Errorf("could not determine current user")
		}

		fmt.Print("Current password: ")
		currentPwd, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read current password: %w", err)
		}
		fmt.Println()

		fmt.Print("New password: ")
		newPwd, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read new password: %w", err)
		}
		fmt.Println()

		fmt.Print("Confirm new password: ")
		confirmPwd, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		fmt.Println()

		if string(newPwd) != string(confirmPwd) {
			return fmt.Errorf("passwords do not match")
		}

		if strings.TrimSpace(string(newPwd)) == "" {
			return fmt.Errorf("new password cannot be empty")
		}

		currentPassword := string(currentPwd)
		pwdReq := api.PasswordChangeRequest{
			CurrentPassword: &currentPassword,
			NewPassword:     string(newPwd),
		}

		if err := m.ChangeUserPassword(me.UserId, pwdReq); err != nil {
			return fmt.Errorf("failed to change password: %w", err)
		}

		fmt.Println("Password changed successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(passwdCmd)
}
