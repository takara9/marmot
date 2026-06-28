package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
)

var userGenerateApiKeyCmd = &cobra.Command{
	Use:   "generate-apikey",
	Short: "Generate an API key for the current user",
	Long:  `Create a new API key for API authentication.`,
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

		comment, _ := cmd.Flags().GetString("comment")

		apiKeyReq := api.ApiKeyCreateRequest{
			Comment: nil,
		}
		if strings.TrimSpace(comment) != "" {
			apiKeyReq.Comment = &comment
		}

		resp, token, err := m.CreateUserApiKey(me.UserId, apiKeyReq)
		if err != nil {
			return fmt.Errorf("failed to create API key: %w", err)
		}

		if resp == nil || token == "" {
			return fmt.Errorf("API key created but no token returned")
		}

		switch outputStyle {
		case "text":
			fmt.Printf("API Key ID: %s\n", resp.Metadata.Name)
			fmt.Printf("API Key Token: %s\n", token)
			fmt.Println("Keep this token safe. You won't be able to view it again.")
		case "json":
			output := map[string]interface{}{
				"id":    resp.Metadata.Name,
				"token": token,
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
		}
		return nil
	},
}

var userListApiKeyCmd = &cobra.Command{
	Use:   "list-apikey",
	Short: "List API keys for the current user",
	Long:  `Display all API keys created by the current user.`,
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

		apiKeys, err := m.ListUserApiKeys(me.UserId)
		if err != nil {
			return fmt.Errorf("failed to list API keys: %w", err)
		}

		switch outputStyle {
		case "text":
			fmt.Println("API Keys:")
			if len(apiKeys) > 0 {
				for _, key := range apiKeys {
					fmt.Printf("  - ID: %s", key.Metadata.Name)
					if key.Spec.Comment != nil && strings.TrimSpace(*key.Spec.Comment) != "" {
						fmt.Printf(" (Comment: %s)", *key.Spec.Comment)
					}
					fmt.Println()
				}
			} else {
				fmt.Println("  (no API keys)")
			}
		case "json":
			jsonBytes, _ := json.MarshalIndent(apiKeys, "", "  ")
			fmt.Println(string(jsonBytes))
		}
		return nil
	},
}

var userDeleteApiKeyCmd = &cobra.Command{
	Use:   "delete-apikey API-KEY-ID",
	Short: "Delete an API key",
	Long:  `Remove an API key from the current user's account.`,
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

		me, err := m.AuthMe()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		if me == nil || strings.TrimSpace(me.UserId) == "" {
			return fmt.Errorf("could not determine current user")
		}

		apiKeyID := strings.TrimSpace(args[0])
		if apiKeyID == "" {
			return fmt.Errorf("API key ID cannot be empty")
		}

		if err := m.DeleteUserApiKey(me.UserId, apiKeyID); err != nil {
			return fmt.Errorf("failed to delete API key: %w", err)
		}

		fmt.Printf("API key '%s' deleted successfully\n", apiKeyID)
		return nil
	},
}

func init() {
	userGenerateApiKeyCmd.Flags().String("comment", "", "Comment for the API key")
	userCmd.AddCommand(userGenerateApiKeyCmd)
	userCmd.AddCommand(userListApiKeyCmd)
	userCmd.AddCommand(userDeleteApiKeyCmd)
}
