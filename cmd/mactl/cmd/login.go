package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login USER-ID",
	Short: "Login to marmotd",
	Long:  `Authenticate with marmotd using userId and password.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := strings.TrimSpace(args[0])
		if userID == "" {
			return fmt.Errorf("userId is required")
		}

		fmt.Print("Password: ")
		pwd, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()
		password := string(pwd)

		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			return err
		}

		previousToken, prevErr := loadAccessToken()
		if prevErr != nil {
			fmt.Fprintln(os.Stderr, "Warning: Failed to load existing token:", prevErr)
			previousToken = ""
		}

		loginResp, err := m.AuthLogin(userID, password)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		if loginResp == nil || strings.TrimSpace(loginResp.AccessToken) == "" {
			return fmt.Errorf("no access token returned")
		}

		previousToken = strings.TrimSpace(previousToken)
		if previousToken != "" && previousToken != strings.TrimSpace(loginResp.AccessToken) {
			oldSessionClient, oldErr := getClientConfig()
			if oldErr != nil {
				fmt.Fprintln(os.Stderr, "Warning: Failed to initialize client for previous session logout:", oldErr)
			} else {
				oldSessionClient.SetAccessToken(previousToken)
				if err := oldSessionClient.AuthLogout(); err != nil {
					fmt.Fprintln(os.Stderr, "Warning: Failed to logout previous session:", err)
				}
			}
		}

		if err := saveAccessToken(loginResp.AccessToken); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}

		switch outputStyle {
		case "text":
			fmt.Printf("Successfully logged in as %s\n", userID)
			if loginResp.MustChangePassword != nil && *loginResp.MustChangePassword {
				fmt.Println("⚠️  You must change your password before using other commands.")
				fmt.Println("   Run: mactl passwd")
			}
		case "json":
			output := map[string]interface{}{
				"success":            true,
				"userId":             userID,
				"mustChangePassword": loginResp.MustChangePassword,
			}
			if loginResp.ExpiresIn != nil {
				output["expiresIn"] = *loginResp.ExpiresIn
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
