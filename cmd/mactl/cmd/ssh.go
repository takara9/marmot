package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
)

var sshExecCommand = exec.Command

var sshCmd = &cobra.Command{
	Use:   "ssh SERVER-NAME [SSH-ARGS...]",
	Short: "Connect to a server via SSH using host-bridge IP",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		serverName := strings.TrimSpace(args[0])
		if serverName == "" {
			return fmt.Errorf("server name is required")
		}

		list, _, err := m.GetServers()
		if err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}

		var servers []api.Server
		if err := json.Unmarshal(list, &servers); err != nil {
			return fmt.Errorf("failed to parse servers: %w", err)
		}

		server, err := findServerByName(servers, serverName)
		if err != nil {
			return err
		}

		targetAddress, err := resolveHostBridgeAddress(*server)
		if err != nil {
			return fmt.Errorf("server %q %w", serverName, err)
		}

		sshArgs := composeSSHArgs(targetAddress, args[1:])
		sshCommand := sshExecCommand("ssh", sshArgs...)
		sshCommand.Stdin = os.Stdin
		sshCommand.Stdout = os.Stdout
		sshCommand.Stderr = os.Stderr

		if err := sshCommand.Run(); err != nil {
			return fmt.Errorf("ssh failed: %w", err)
		}
		return nil
	},
}

func findServerByName(servers []api.Server, name string) (*api.Server, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, fmt.Errorf("server name is required")
	}

	for i := range servers {
		if strings.TrimSpace(servers[i].Metadata.Name) == trimmedName {
			return &servers[i], nil
		}
	}

	return nil, fmt.Errorf("server %q not found", trimmedName)
}

func resolveHostBridgeAddress(server api.Server) (string, error) {
	if server.Spec.NetworkInterface == nil || len(*server.Spec.NetworkInterface) == 0 {
		return "", fmt.Errorf("is not connected to host-bridge")
	}

	for _, nic := range *server.Spec.NetworkInterface {
		if strings.TrimSpace(nic.Networkname) != "host-bridge" {
			continue
		}
		if nic.Address == nil || strings.TrimSpace(*nic.Address) == "" {
			return "", fmt.Errorf("has no static address on host-bridge")
		}
		return strings.TrimSpace(*nic.Address), nil
	}

	return "", fmt.Errorf("is not connected to host-bridge")
}

func composeSSHArgs(targetAddress string, extraArgs []string) []string {
	args := make([]string, 0, len(extraArgs)+1)
	separator := -1
	for i, arg := range extraArgs {
		if arg == "--" {
			separator = i
			break
		}
	}

	if separator >= 0 {
		args = append(args, extraArgs[:separator]...)
		args = append(args, targetAddress)
		args = append(args, extraArgs[separator+1:]...)
		return args
	}

	args = append(args, extraArgs...)
	args = append(args, targetAddress)
	return args
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
