package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var userAddRoleCmd = &cobra.Command{
	Use:   "add-role USER-ID ROLE-NAME",
	Short: "Add a role to a user (admin only)",
	Long:  `Assign a role to a user account.`,
	Args:  cobra.ExactArgs(2),
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
		roleName := strings.TrimSpace(args[1])

		if userID == "" || roleName == "" {
			return fmt.Errorf("user ID and role name cannot be empty")
		}

		if err := m.AddUserRole(userID, roleName); err != nil {
			return fmt.Errorf("failed to add role: %w", err)
		}

		fmt.Printf("Role '%s' added to user '%s' successfully\n", roleName, userID)
		return nil
	},
}

var userDelRoleCmd = &cobra.Command{
	Use:   "del-role USER-ID ROLE-NAME",
	Short: "Remove a role from a user (admin only)",
	Long:  `Unassign a role from a user account.`,
	Args:  cobra.ExactArgs(2),
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
		roleName := strings.TrimSpace(args[1])

		if userID == "" || roleName == "" {
			return fmt.Errorf("user ID and role name cannot be empty")
		}

		if err := m.DeleteUserRole(userID, roleName); err != nil {
			return fmt.Errorf("failed to delete role: %w", err)
		}

		fmt.Printf("Role '%s' removed from user '%s' successfully\n", roleName, userID)
		return nil
	},
}

var userListRoleCmd = &cobra.Command{
	Use:   "list-role USER-ID",
	Short: "List roles assigned to a user",
	Long:  `Display all roles assigned to a specified user.`,
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

		roles, err := m.ListUserRoles(userID)
		if err != nil {
			return fmt.Errorf("failed to list roles: %w", err)
		}

		fmt.Printf("Roles for user '%s':\n", userID)
		if len(roles) > 0 {
			for _, role := range roles {
				fmt.Printf("  - %s\n", role)
			}
		} else {
			fmt.Println("  (no roles assigned)")
		}
		return nil
	},
}

func init() {
	userCmd.AddCommand(userAddRoleCmd)
	userCmd.AddCommand(userDelRoleCmd)
	userCmd.AddCommand(userListRoleCmd)
}
