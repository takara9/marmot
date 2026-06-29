package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var userAddCmd = &cobra.Command{
	Use:   "add USER-ID",
	Short: "Add a new user (admin only)",
	Long:  `Create a new user with specified user ID and optional role assignment.`,
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

		roleStr, _ := cmd.Flags().GetString("role")
		passwdStr, _ := cmd.Flags().GetString("passwd")

		if strings.TrimSpace(passwdStr) == "" {
			fmt.Print("Password: ")
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

		passwordHash, err := passwordHashFromPlain(passwdStr)
		if err != nil {
			return err
		}

		enabled := true
		user := api.User{
			ApiVersion: "v1",
			Kind:       "User",
			Metadata: api.Metadata{
				Name: userID,
			},
			Spec: api.UserSpec{
				Enabled:      enabled,
				PasswordHash: util.StringPtr(passwordHash),
			},
		}

		resp, err := m.CreateUser(user)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		if resp == nil {
			return fmt.Errorf("user created but no response")
		}

		if strings.TrimSpace(roleStr) != "" {
			if err := m.AddUserRole(userID, roleStr); err != nil {
				return fmt.Errorf("user created but failed to assign role: %w", err)
			}
		}

		fmt.Printf("User '%s' created successfully\n", userID)
		if strings.TrimSpace(roleStr) != "" {
			fmt.Printf("Role '%s' assigned\n", roleStr)
		}
		return nil
	},
}

func passwordHashFromPlain(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

func init() {
	userAddCmd.Flags().String("role", "", "Role to assign to the user")
	userAddCmd.Flags().String("passwd", "", "Password (if not provided, will prompt)")
	userCmd.AddCommand(userAddCmd)
}
