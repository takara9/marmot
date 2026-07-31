package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"go.yaml.in/yaml/v3"
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

		if _, _, err := m.StopServerById(serverID); err != nil {
			return fmt.Errorf("failed to stop server %q for restart: %w", serverName, err)
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

func printLifecycleResult(byteBody []byte, textMessage string) error {
	switch outputStyle {
	case "text":
		var data interface{}
		if err := json.Unmarshal(byteBody, &data); err != nil {
			return err
		}
		fmt.Println(textMessage, data.(map[string]interface{})["id"])
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