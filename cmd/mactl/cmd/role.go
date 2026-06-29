package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var roleCmd = &cobra.Command{
	Use:   "role",
	Aliases: []string{"whoami"},
	Short: "Show your roles",
	Long:  `Display the roles assigned to the currently logged-in user.`,
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

		if me == nil {
			return fmt.Errorf("could not get user information")
		}

		switch outputStyle {
		case "text":
			fmt.Printf("User: %s\n", me.UserId)
			if me.Roles != nil && len(me.Roles) > 0 {
				fmt.Println("Roles:")
				for _, role := range me.Roles {
					fmt.Printf("  - %s\n", role)
				}
			} else {
				fmt.Println("No roles assigned")
			}
		case "json":
			output := map[string]interface{}{
				"userId": me.UserId,
				"roles":  me.Roles,
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
		case "yaml":
			output := map[string]interface{}{
				"userId": me.UserId,
				"roles":  me.Roles,
			}
			yamlBytes, _ := yaml.Marshal(output)
			fmt.Println(string(yamlBytes))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(roleCmd)
}
