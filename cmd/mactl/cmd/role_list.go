package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var roleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available roles",
	Long:  `Display all built-in roles defined in marmotd.`,
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

		roles, err := m.ListRoles()
		if err != nil {
			return fmt.Errorf("failed to list roles: %w", err)
		}

		sort.Slice(roles, func(i, j int) bool {
			return strings.TrimSpace(roles[i].Metadata.Name) < strings.TrimSpace(roles[j].Metadata.Name)
		})

		if len(roles) == 0 {
			fmt.Println("No roles defined")
			return nil
		}

		fmt.Printf("%-32s  %s\n", "ROLE-NAME", "DESCRIPTION")
		for _, r := range roles {
			name := strings.TrimSpace(r.Metadata.Name)
			desc := ""
			if r.Spec.Description != nil {
				desc = strings.TrimSpace(*r.Spec.Description)
			}
			fmt.Printf("%-32s  %s\n", name, desc)
		}
		return nil
	},
}

func init() {
	roleCmd.AddCommand(roleListCmd)
}
