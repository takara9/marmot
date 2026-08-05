package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

const (
	serverLifecycleWaitTimeout      = 10 * time.Minute
	serverLifecycleWaitPollInterval = 5 * time.Second
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start resources",
}

var startServerCmd = &cobra.Command{
	Use:   "server SERVER-NAME",
	Short: "Start a server by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		serverID, serverName, err := resolveServerIDByName(m, args[0])
		if err != nil {
			return err
		}

		statusCode, err := getServerStatusCode(m, serverID)
		if err != nil {
			return err
		}
		if isStartNoopStatus(statusCode) {
			return printLifecycleResultFromID(serverID, "サーバーが起動されました。ID:")
		}

		byteBody, _, err := m.StartServerById(serverID)
		if err != nil {
			return fmt.Errorf("failed to start server %q: %w", serverName, err)
		}

		return printLifecycleResult(byteBody, "サーバーが起動されました。ID:")
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop resources",
}

var stopServerCmd = &cobra.Command{
	Use:   "server SERVER-NAME",
	Short: "Stop a server by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		serverID, serverName, err := resolveServerIDByName(m, args[0])
		if err != nil {
			return err
		}

		statusCode, err := getServerStatusCode(m, serverID)
		if err != nil {
			return err
		}
		if isStopNoopStatus(statusCode) {
			return printLifecycleResultFromID(serverID, "サーバーが停止されました。ID:")
		}

		byteBody, _, err := m.StopServerById(serverID)
		if err != nil {
			return fmt.Errorf("failed to stop server %q: %w", serverName, err)
		}

		return printLifecycleResult(byteBody, "サーバーが停止されました。ID:")
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart resources",
}

var restartServerCmd = &cobra.Command{
	Use:   "server SERVER-NAME",
	Short: "Restart a server by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		serverID, serverName, err := resolveServerIDByName(m, args[0])
		if err != nil {
			return err
		}

		statusCode, err := getServerStatusCode(m, serverID)
		if err != nil {
			return err
		}

		if !isStopNoopStatus(statusCode) {
			if _, _, err := m.StopServerById(serverID); err != nil {
				return fmt.Errorf("failed to stop server %q for restart: %w", serverName, err)
			}
		}

		if err := waitForServerStatus(m, serverID, db.SERVER_STOPPED, serverLifecycleWaitTimeout, serverLifecycleWaitPollInterval); err != nil {
			return fmt.Errorf("failed to wait for server %q to stop for restart: %w", serverName, err)
		}

		byteBody, _, err := m.StartServerById(serverID)
		if err != nil {
			return fmt.Errorf("failed to start server %q for restart: %w", serverName, err)
		}

		return printLifecycleResult(byteBody, "サーバーが再起動されました。ID:")
	},
}

func resolveServerIDByName(m *client.MarmotEndpoint, name string) (string, string, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return "", "", fmt.Errorf("server name is required")
	}

	list, _, err := m.GetServers()
	if err != nil {
		return "", "", fmt.Errorf("failed to list servers: %w", err)
	}

	var servers []api.Server
	if err := json.Unmarshal(list, &servers); err != nil {
		return "", "", fmt.Errorf("failed to parse servers: %w", err)
	}

	server, err := findServerByName(servers, trimmedName)
	if err != nil {
		return "", "", err
	}

	return string(api.ServerID(*server)), trimmedName, nil
}

func getServerStatusCode(m *client.MarmotEndpoint, serverID string) (int, error) {
	body, _, err := m.GetServerById(serverID)
	if err != nil {
		return 0, fmt.Errorf("failed to get server %s: %w", serverID, err)
	}

	var server api.Server
	if err := json.Unmarshal(body, &server); err != nil {
		return 0, fmt.Errorf("failed to parse server %s: %w", serverID, err)
	}
	if server.Status == nil {
		return 0, nil
	}

	return server.Status.StatusCode, nil
}

func isStartNoopStatus(statusCode int) bool {
	return statusCode == db.SERVER_RUNNING || statusCode == db.SERVER_STARTING
}

func isStopNoopStatus(statusCode int) bool {
	return statusCode == db.SERVER_STOPPED || statusCode == db.SERVER_STOPPING
}

func waitForServerStatus(m *client.MarmotEndpoint, serverID string, expectedStatus int, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		body, _, err := m.GetServerById(serverID)
		if err != nil {
			lastErr = err
		} else {
			var server api.Server
			if err := json.Unmarshal(body, &server); err != nil {
				lastErr = fmt.Errorf("failed to parse server %s while waiting for %s: %w", serverID, db.ServerStatus[expectedStatus], err)
			} else if server.Status != nil {
				if server.Status.StatusCode == expectedStatus {
					return nil
				}
				if server.Status.StatusCode == db.SERVER_ERROR {
					message := "server status became ERROR"
					if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
						message = strings.TrimSpace(*server.Status.Message)
					}
					return fmt.Errorf("server %s is ERROR while waiting for %s: %s", serverID, db.ServerStatus[expectedStatus], message)
				}
			}
		}

		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timeout waiting for server %s to become %s: last error: %w", serverID, db.ServerStatus[expectedStatus], lastErr)
			}
			return fmt.Errorf("timeout waiting for server %s to become %s", serverID, db.ServerStatus[expectedStatus])
		}
		time.Sleep(interval)
	}
}

func printLifecycleResultFromID(serverID string, textMessage string) error {
	return printLifecycleResult([]byte(fmt.Sprintf(`{"id":%q}`, serverID)), textMessage)
}

func printLifecycleResult(byteBody []byte, textMessage string) error {
	switch outputStyle {
	case "text":
		id, err := extractResponseID(byteBody)
		if err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		displayID := id
		if displayID == "" {
			displayID = "<nil>"
		}
		fmt.Println(textMessage, displayID)
		return nil
	case "json":
		fmt.Println(string(byteBody))
		return nil
	case "yaml":
		var data interface{}
		if err := json.Unmarshal(byteBody, &data); err != nil {
			return err
		}
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		fmt.Println(string(yamlBytes))
		return nil
	default:
		return fmt.Errorf("output style must set text/json/yaml")
	}
}

func init() {
	startCmd.AddCommand(startServerCmd)
	stopCmd.AddCommand(stopServerCmd)
	restartCmd.AddCommand(restartServerCmd)

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
}