package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"gopkg.in/yaml.v3"
)

type sessionRow struct {
	UserID     string `json:"userId" yaml:"userId"`
	SessionID  string `json:"sessionId" yaml:"sessionId"`
	Status     string `json:"status" yaml:"status"`
	From       string `json:"from" yaml:"from"`
	Comment    string `json:"comment" yaml:"comment"`
	IssuedAt   string `json:"issuedAt" yaml:"issuedAt"`
	LastUsedAt string `json:"lastUsedAt" yaml:"lastUsedAt"`
	Age        string `json:"age" yaml:"age"`
}

var userSessionCmd = &cobra.Command{
	Use:   "session",
	Short: "List login sessions",
	Long:  `List login session API keys. Administrators can see every user's sessions; other users see only their own sessions.`,
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

		rows, err := buildSessionRows(m, me)
		if err != nil {
			return err
		}

		if len(rows) == 0 {
			fmt.Println("No login sessions found")
			return nil
		}

		sort.Slice(rows, func(i, j int) bool {
			if rows[i].UserID != rows[j].UserID {
				return rows[i].UserID < rows[j].UserID
			}
			if rows[i].IssuedAt != rows[j].IssuedAt {
				return rows[i].IssuedAt > rows[j].IssuedAt
			}
			return rows[i].SessionID < rows[j].SessionID
		})

		switch outputStyle {
		case "json":
			printSessionRowsJSON(rows)
		case "yaml":
			printSessionRowsYAML(rows)
		default:
			printSessionRowsText(rows)
		}

		return nil
	},
}

func buildSessionRows(m *client.MarmotEndpoint, me *api.AuthMe) ([]sessionRow, error) {
	if isAdministratorAuthMe(me) {
		return collectAllUserSessionRows(m)
	}
	return collectUserSessionRows(m, strings.TrimSpace(me.UserId))
}

func collectAllUserSessionRows(m *client.MarmotEndpoint) ([]sessionRow, error) {
	users, err := m.ListUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	rows := make([]sessionRow, 0)
	for _, user := range users {
		userID := strings.TrimSpace(user.Metadata.Id)
		if userID == "" {
			userID = strings.TrimSpace(user.Metadata.Name)
		}
		if userID == "" {
			continue
		}

		userRows, err := collectUserSessionRows(m, userID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, userRows...)
	}

	return rows, nil
}

func collectUserSessionRows(m *client.MarmotEndpoint, userID string) ([]sessionRow, error) {
	keys, err := m.ListUserApiKeys(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list session API keys for user '%s': %w", userID, err)
	}

	rows := make([]sessionRow, 0, len(keys))
	for _, key := range keys {
		if !isLoginSessionApiKey(key) {
			continue
		}
		rows = append(rows, sessionRow{
			UserID:     userID,
			SessionID:  apiKeyIdentifier(key),
			Status:     sessionStatusText(key),
			From:       sessionFromIPText(key),
			Comment:    sessionCommentText(key),
			IssuedAt:   formatTimeText(key.Spec.IssuedAt),
			LastUsedAt: formatTimeText(sessionLastUsedAt(key)),
			Age:        timeSinceText(key.Spec.IssuedAt),
		})
	}

	return rows, nil
}

func isAdministratorAuthMe(me *api.AuthMe) bool {
	if me == nil {
		return false
	}
	for _, role := range me.Roles {
		if strings.EqualFold(strings.TrimSpace(role), "Administrator") {
			return true
		}
	}
	return false
}

func isLoginSessionApiKey(apiKey api.ApiKey) bool {
	if apiKey.Spec.SessionType != nil && strings.EqualFold(strings.TrimSpace(*apiKey.Spec.SessionType), "login") {
		return true
	}
	return apiKey.Spec.Comment != nil && strings.TrimSpace(*apiKey.Spec.Comment) == "login-session"
}

func apiKeyIdentifier(apiKey api.ApiKey) string {
	id := strings.TrimSpace(apiKey.Metadata.Id)
	if id != "" {
		return id
	}
	return strings.TrimSpace(apiKey.Metadata.Name)
}

func sessionCommentText(apiKey api.ApiKey) string {
	if apiKey.Spec.Comment == nil {
		return "-"
	}
	comment := strings.TrimSpace(*apiKey.Spec.Comment)
	if comment == "" {
		return "-"
	}
	return comment
}

func sessionFromIPText(apiKey api.ApiKey) string {
	if apiKey.Spec.FromIP == nil {
		return "-"
	}
	from := strings.TrimSpace(*apiKey.Spec.FromIP)
	if from == "" {
		return "-"
	}
	return from
}

func sessionStatusText(apiKey api.ApiKey) string {
	if apiKey.Spec.Revoked != nil && *apiKey.Spec.Revoked {
		return "revoked"
	}
	if apiKey.Status != nil && apiKey.Status.RevokedAt != nil {
		return "revoked"
	}
	if apiKey.Spec.ExpiresAt != nil && time.Now().After(*apiKey.Spec.ExpiresAt) {
		return "expired"
	}
	return "active"
}

func sessionLastUsedAt(apiKey api.ApiKey) *time.Time {
	if apiKey.Status == nil {
		return nil
	}
	return apiKey.Status.LastUsedAt
}

func formatTimeText(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func timeSinceText(t *time.Time) string {
	if t == nil {
		return "-"
	}
	elapsed := time.Since(*t)
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

func printSessionRowsText(rows []sessionRow) {
	fmt.Printf("%-24s  %-8s  %-8s  %-39s  %-25s  %-8s  %-20s\n", "USER-ID", "SESSION-ID", "STATUS", "FROM", "LAST-USED", "AGE", "COMMENT")
	for _, row := range rows {
		fmt.Printf("%-24s  %-8s  %-8s  %-39s  %-25s  %-8s  %-20s\n", row.UserID, shortSessionID(row.SessionID), row.Status, row.From, row.LastUsedAt, row.Age, row.Comment)
	}
}

func shortSessionID(sessionID string) string {
	id := strings.TrimSpace(sessionID)
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func printSessionRowsJSON(rows []sessionRow) {
	jsonBytes, _ := json.MarshalIndent(rows, "", "  ")
	fmt.Println(string(jsonBytes))
}

func printSessionRowsYAML(rows []sessionRow) {
	yamlBytes, _ := yaml.Marshal(rows)
	fmt.Println(string(yamlBytes))
}

func init() {
	userCmd.AddCommand(userSessionCmd)
}