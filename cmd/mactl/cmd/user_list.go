package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
)

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered users",
	Long:  `Display all users registered in marmotd.`,
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

		users, err := m.ListUsers()
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		sort.Slice(users, func(i, j int) bool {
			left := strings.TrimSpace(users[i].Metadata.Id)
			if left == "" {
				left = strings.TrimSpace(users[i].Metadata.Name)
			}
			right := strings.TrimSpace(users[j].Metadata.Id)
			if right == "" {
				right = strings.TrimSpace(users[j].Metadata.Name)
			}
			return left < right
		})

		if len(users) == 0 {
			fmt.Println("No users registered")
			return nil
		}

		fmt.Printf("%-24s  %-8s  %-36s  %-8s\n", "USER-ID", "ENABLED", "ROLE", "AGE")
		for _, u := range users {
			userID := strings.TrimSpace(u.Metadata.Id)
			if userID == "" {
				userID = strings.TrimSpace(u.Metadata.Name)
			}
			fmt.Printf("%-24s  %-8t  %-36s  %-8s\n", userID, u.Spec.Enabled, userRoleText(u), userAgeText(u))
		}

		return nil
	},
}

func userRoleText(u api.User) string {
	if u.Spec.Roles == nil || len(*u.Spec.Roles) == 0 {
		return "-"
	}
	roles := make([]string, 0, len(*u.Spec.Roles))
	for _, role := range *u.Spec.Roles {
		trimmed := strings.TrimSpace(role)
		if trimmed == "" {
			continue
		}
		roles = append(roles, trimmed)
	}
	if len(roles) == 0 {
		return "-"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

func userAgeText(u api.User) string {
	if u.Status == nil || u.Status.PasswordUpdatedAt == nil {
		return "-"
	}
	elapsed := time.Since(*u.Status.PasswordUpdatedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(elapsed/(24*time.Hour)))
	}
	if elapsed >= time.Hour {
		return fmt.Sprintf("%dh", int(elapsed/time.Hour))
	}
	if elapsed >= time.Minute {
		return fmt.Sprintf("%dm", int(elapsed/time.Minute))
	}
	return fmt.Sprintf("%ds", int(elapsed/time.Second))
}

func init() {
	userCmd.AddCommand(userListCmd)
}
