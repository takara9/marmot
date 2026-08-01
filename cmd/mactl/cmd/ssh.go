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
	Use:   "ssh [USER@]SERVER-NAME -- [SSH-ARGS...]",
	Short: "Connect to a server via SSH using host-bridge IP",
	// Treat all ssh-style flags as positional args so options like -tt pass through unchanged.
	DisableFlagParsing: true,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		sshArgs := args
		if len(sshArgs) > 0 && strings.TrimSpace(sshArgs[0]) == "ssh" {
			sshArgs = sshArgs[1:]
		}
		if len(sshArgs) == 0 {
			return fmt.Errorf("ssh target is required")
		}

		list, _, err := m.GetServers()
		if err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}

		var servers []api.Server
		if err := json.Unmarshal(list, &servers); err != nil {
			return fmt.Errorf("failed to parse servers: %w", err)
		}

		sshArgs, err = rewriteSSHArgsForMarmot(servers, sshArgs)
		if err != nil {
			return err
		}

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

func rewriteSSHArgsForMarmot(servers []api.Server, sshArgs []string) ([]string, error) {
	// Backward-compatible support for mactl syntax:
	//   mactl ssh SERVER -- [SSH-ARGS...]
	// and for SERVER followed by ssh options without `--`.
	if len(sshArgs) >= 2 && !strings.HasPrefix(sshArgs[0], "-") && (sshArgs[1] == "--" || strings.HasPrefix(sshArgs[1], "-")) {
		target := sshArgs[0]
		extra := sshArgs[1:]
		if extra[0] == "--" {
			extra = extra[1:]
		}

		sep := -1
		for i, arg := range extra {
			if arg == "--" {
				sep = i
				break
			}
		}
		if sep >= 0 {
			sshArgs = append(append(extra[:sep], target), extra[sep+1:]...)
		} else {
			sshArgs = append(extra, target)
		}
	}

	connectIdx, err := findSSHConnectTargetIndex(sshArgs)
	if err != nil {
		return nil, err
	}

	connectArg := strings.TrimSpace(sshArgs[connectIdx])
	if connectArg == "" {
		return nil, fmt.Errorf("ssh target is required")
	}
	sshUser, serverName, err := parseSSHLoginTarget(connectArg)
	if err != nil {
		if strings.HasPrefix(connectArg, "-") {
			return nil, fmt.Errorf("ssh target is required")
		}
		return nil, err
	}

	server, err := findServerByName(servers, serverName)
	if err != nil {
		return nil, err
	}

	targetAddress, err := resolveHostBridgeAddress(*server)
	if err != nil {
		return nil, fmt.Errorf("server %q %w", serverName, err)
	}

	sshArgs[connectIdx] = buildSSHTargetAddress(sshUser, targetAddress)
	return sshArgs, nil
}

func parseSSHLoginTarget(value string) (string, string, error) {
	target := strings.TrimSpace(value)
	if target == "" {
		return "", "", fmt.Errorf("server name is required")
	}
	if !strings.Contains(target, "@") {
		return "", target, nil
	}

	parts := strings.SplitN(target, "@", 2)
	user := strings.TrimSpace(parts[0])
	serverName := strings.TrimSpace(parts[1])
	if user == "" || serverName == "" {
		return "", "", fmt.Errorf("invalid ssh target %q; use [USER@]SERVER-NAME", target)
	}
	return user, serverName, nil
}

func buildSSHTargetAddress(user, ipAddress string) string {
	if strings.TrimSpace(user) == "" {
		return strings.TrimSpace(ipAddress)
	}
	return strings.TrimSpace(user) + "@" + strings.TrimSpace(ipAddress)
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

func findSSHConnectTargetIndex(args []string) (int, error) {
	if len(args) == 0 {
		return -1, fmt.Errorf("ssh target is required")
	}

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return 0, nil
	}

	consumesNext := map[string]bool{
		"-b": true,
		"-c": true,
		"-D": true,
		"-E": true,
		"-F": true,
		"-I": true,
		"-i": true,
		"-J": true,
		"-L": true,
		"-l": true,
		"-m": true,
		"-O": true,
		"-o": true,
		"-p": true,
		"-Q": true,
		"-R": true,
		"-S": true,
		"-W": true,
		"-w": true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 >= len(args) {
				return -1, fmt.Errorf("ssh target is required")
			}
			return i + 1, nil
		}
		if !strings.HasPrefix(arg, "-") {
			return i, nil
		}
		if len(arg) > 2 && (strings.HasPrefix(arg, "-o") || strings.HasPrefix(arg, "-i") || strings.HasPrefix(arg, "-l") || strings.HasPrefix(arg, "-p") || strings.HasPrefix(arg, "-J") || strings.HasPrefix(arg, "-W") || strings.HasPrefix(arg, "-L") || strings.HasPrefix(arg, "-R") || strings.HasPrefix(arg, "-S") || strings.HasPrefix(arg, "-b") || strings.HasPrefix(arg, "-c") || strings.HasPrefix(arg, "-D") || strings.HasPrefix(arg, "-E") || strings.HasPrefix(arg, "-F") || strings.HasPrefix(arg, "-I") || strings.HasPrefix(arg, "-m") || strings.HasPrefix(arg, "-Q") || strings.HasPrefix(arg, "-w")) {
			continue
		}
		if consumesNext[arg] {
			i++
			if i >= len(args) {
				return -1, fmt.Errorf("ssh target is required")
			}
		}
	}

	return -1, fmt.Errorf("ssh target is required")
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
