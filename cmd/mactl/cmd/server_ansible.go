package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/db"
)

const (
	serverAnsibleDefaultUser       = "root"
	serverAnsibleWaitTimeout       = 10 * time.Minute
	serverAnsibleWaitPollInterval  = 5 * time.Second
	serverAnsiblePingTimeout       = 3 * time.Minute
	serverAnsiblePingPollInterval  = 5 * time.Second
	serverAnsiblePrivateKeyEnvName = "MARMOT_ANSIBLE_PRIVATE_KEY"
)

var serverAnsibleExecCommand = exec.Command

func maybeApplyServerAnsiblePlaybook(m *client.MarmotEndpoint, server api.Server, createResponse []byte) error {
	if server.Spec.Ansible == nil {
		return nil
	}

	if err := validateServerAnsibleSpec(server); err != nil {
		return err
	}

	if isServerAnsibleOnBootEnabled(server.Spec.Ansible) {
		// onBoot モードは cloud-init 側で実行するため、mactl からは適用しない。
		return nil
	}

	serverID, err := extractSuccessID(createResponse)
	if err != nil {
		return fmt.Errorf("failed to parse create response id for ansible apply: %w", err)
	}

	fmt.Fprintln(os.Stderr, "OS起動待機中.....")
	if err := waitServerRunning(m, serverID, serverAnsibleWaitTimeout, serverAnsibleWaitPollInterval); err != nil {
		return err
	}

	targetAddress, err := waitServerHostBridgeAddress(m, serverID, serverAnsiblePingTimeout, serverAnsiblePingPollInterval)
	if err != nil {
		return err
	}

	playbookPath, err := resolveServerAnsiblePlaybookPath(optionalTrimmedString(server.Spec.Ansible.Playbook))
	if err != nil {
		return err
	}
	inventoryPath, err := resolveServerAnsibleInventoryPath(optionalTrimmedString(server.Spec.Ansible.Inventory))
	if err != nil {
		return err
	}
	privateKeyPaths, err := resolveServerAnsiblePrivateKeyPaths()
	if err != nil {
		return err
	}

	if err := waitServerAnsiblePingReady(targetAddress, privateKeyPaths, serverAnsiblePingTimeout, serverAnsiblePingPollInterval); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "playbook 適用開始.....")
	if err := runServerAnsiblePlaybook(playbookPath, inventoryPath, privateKeyPaths, server.Spec.Ansible.ExtraArgs); err != nil {
		return err
	}
	return nil
}

func validateServerAnsibleSpec(server api.Server) error {
	if server.Spec.Ansible == nil {
		return nil
	}
	playbook := optionalTrimmedString(server.Spec.Ansible.Playbook)
	inventory := optionalTrimmedString(server.Spec.Ansible.Inventory)
	remotePlaybook := ""
	if server.Spec.Ansible.RemotePlaybook != nil {
		remotePlaybook = strings.TrimSpace(*server.Spec.Ansible.RemotePlaybook)
	}
	pullURL := ""
	if server.Spec.Ansible.Pull != nil && server.Spec.Ansible.Pull.Url != nil {
		pullURL = strings.TrimSpace(*server.Spec.Ansible.Pull.Url)
	}

	if isServerAnsibleOnBootEnabled(server.Spec.Ansible) {
		if remotePlaybook == "" && pullURL == "" {
			return fmt.Errorf("spec.ansible.remotePlaybook or spec.ansible.pull.url is required when spec.ansible.onBoot is true")
		}
		if remotePlaybook != "" && pullURL != "" {
			return fmt.Errorf("spec.ansible.remotePlaybook cannot be combined with spec.ansible.pull.url")
		}
		if playbook != "" || inventory != "" {
			return fmt.Errorf("spec.ansible.onBoot=true cannot be combined with spec.ansible.playbook or spec.ansible.inventory")
		}
		return nil
	}

	if remotePlaybook != "" {
		return fmt.Errorf("spec.ansible.remotePlaybook requires spec.ansible.onBoot=true")
	}
	if pullURL != "" {
		return fmt.Errorf("spec.ansible.pull.url requires spec.ansible.onBoot=true")
	}

	if playbook == "" {
		return fmt.Errorf("spec.ansible.playbook is required when spec.ansible is set")
	}
	if inventory == "" {
		return fmt.Errorf("spec.ansible.inventory is required when spec.ansible is set")
	}
	if server.Spec.NetworkInterface == nil || len(*server.Spec.NetworkInterface) == 0 {
		return fmt.Errorf("spec.ansible requires spec.networkInterface with host-bridge")
	}
	for _, nic := range *server.Spec.NetworkInterface {
		if strings.TrimSpace(nic.Networkname) != "host-bridge" {
			continue
		}
		return nil
	}
	return fmt.Errorf("spec.ansible can be used only when host-bridge is specified in spec.networkInterface")
}

func isServerAnsibleOnBootEnabled(ansible *api.ServerAnsible) bool {
	return ansible != nil && ansible.OnBoot != nil && *ansible.OnBoot
}

func optionalTrimmedString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func waitServerHostBridgeAddress(m *client.MarmotEndpoint, serverID string, timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		body, _, err := m.GetServerById(serverID)
		if err != nil {
			lastErr = fmt.Errorf("failed to get server %s while waiting for host-bridge address: %w", serverID, err)
		} else {
			var srv api.Server
			if err := json.Unmarshal(body, &srv); err != nil {
				lastErr = fmt.Errorf("failed to parse server %s while waiting for host-bridge address: %w", serverID, err)
			} else {
				addr, err := hostBridgeAddressFromServer(srv)
				if err == nil {
					return addr, nil
				}
				lastErr = err
			}
		}

		if time.Now().After(deadline) {
			if lastErr != nil {
				return "", fmt.Errorf("timeout waiting for host-bridge address for server %s: %w", serverID, lastErr)
			}
			return "", fmt.Errorf("timeout waiting for host-bridge address for server %s", serverID)
		}
		time.Sleep(interval)
	}
}

func hostBridgeAddressFromServer(server api.Server) (string, error) {
	if server.Spec.NetworkInterface == nil || len(*server.Spec.NetworkInterface) == 0 {
		return "", fmt.Errorf("spec.networkInterface with host-bridge is not set")
	}
	for _, nic := range *server.Spec.NetworkInterface {
		if strings.TrimSpace(nic.Networkname) != "host-bridge" {
			continue
		}
		if nic.Address == nil || strings.TrimSpace(*nic.Address) == "" {
			return "", fmt.Errorf("host-bridge address is not assigned yet")
		}
		return strings.TrimSpace(*nic.Address), nil
	}
	return "", fmt.Errorf("host-bridge is not set in spec.networkInterface")
}

func extractSuccessID(body []byte) (string, error) {
	id, err := extractResponseID(body)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("id is empty")
	}
	return id, nil
}

func waitServerRunning(m *client.MarmotEndpoint, serverID string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		body, _, err := m.GetServerById(serverID)
		if err != nil {
			lastErr = err
			if strings.Contains(err.Error(), "IDが存在しません") {
				return fmt.Errorf("server %s is not found before ansible apply: %w", serverID, err)
			}
		} else {
			lastErr = nil
		}
		if err == nil {
			var srv api.Server
			if err := json.Unmarshal(body, &srv); err == nil {
				if srv.Status != nil {
					if srv.Status.StatusCode == db.SERVER_RUNNING {
						return nil
					}
					if srv.Status.StatusCode == db.SERVER_ERROR {
						msg := ""
						if srv.Status.Message != nil {
							msg = strings.TrimSpace(*srv.Status.Message)
						}
						if msg == "" {
							msg = "server status became ERROR"
						}
						return fmt.Errorf("server %s is ERROR before ansible apply: %s", serverID, msg)
					}
				}
			}
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timeout waiting for server %s to become RUNNING: last error: %w", serverID, lastErr)
			}
			return fmt.Errorf("timeout waiting for server %s to become RUNNING", serverID)
		}
		time.Sleep(interval)
	}
}

func resolveServerAnsiblePlaybookPath(path string) (string, error) {
	return resolveServerAnsibleFilePath(path, "playbook")
}

func resolveServerAnsibleInventoryPath(path string) (string, error) {
	return resolveServerAnsibleFilePath(path, "inventory")
}

func resolveServerAnsibleFilePath(path, field string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("spec.ansible.%s is empty", field)
	}
	if !filepath.IsAbs(p) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		p = filepath.Join(cwd, p)
	}
	if info, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("ansible %s file is not found: %s", field, p)
	} else if info.IsDir() {
		return "", fmt.Errorf("ansible %s path is a directory: %s", field, p)
	}
	return p, nil
}

// resolveServerAnsiblePrivateKeyPaths は、Ansible疎通に使う秘密鍵の候補を返す。
// 環境変数が設定されている場合はそれを唯一の候補として使う。
// 未設定の場合は ~/.ssh に存在する鍵を全て候補として返し、SSH自身に
// (IdentitiesOnly=yes の下で)有効な鍵を選ばせる。こうすることで、VM側に認可された
// 鍵の種類(id_rsa/id_ed25519等)を mactl 側が事前に知らなくても疎通できる(issue #723)。
func resolveServerAnsiblePrivateKeyPaths() ([]string, error) {
	if p := strings.TrimSpace(os.Getenv(serverAnsiblePrivateKeyEnvName)); p != "" {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%s points to missing key: %s", serverAnsiblePrivateKeyEnvName, p)
		}
		return []string{p}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}
	found := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, candidate)
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no private key found; set %s or place key under ~/.ssh", serverAnsiblePrivateKeyEnvName)
	}
	return found, nil
}

func waitServerAnsiblePingReady(targetAddress string, privateKeyPaths []string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := runServerAnsiblePing(targetAddress, privateKeyPaths); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for ansible ping to %s: %w", targetAddress, lastErr)
		}
		time.Sleep(interval)
	}
}

func runServerAnsiblePing(targetAddress string, privateKeyPaths []string) error {
	args := []string{
		"all",
		"-i", targetAddress + ",",
		"-m", "ping",
	}
	args = append(args, serverAnsiblePrivateKeyCLIArgs(privateKeyPaths)...)
	cmd := serverAnsibleExecCommand("ansible", args...)
	cmd.Env = serverAnsibleCommandEnv(privateKeyPaths)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("ansible ping failed: %w", err)
		}
		return fmt.Errorf("ansible ping failed: %w: %s", err, trimmed)
	}
	return nil
}

func runServerAnsiblePlaybook(playbookPath, inventoryPath string, privateKeyPaths []string, extraArgs *[]string) error {
	args := []string{
		"-i", inventoryPath,
		playbookPath,
	}
	args = append(args, serverAnsiblePrivateKeyCLIArgs(privateKeyPaths)...)
	if extraArgs != nil {
		expandedArgs, err := expandServerAnsibleExtraArgs(*extraArgs)
		if err != nil {
			return err
		}
		args = append(args, expandedArgs...)
	}
	cmd := serverAnsibleExecCommand("ansible-playbook", args...)
	cmd.Env = serverAnsibleCommandEnv(privateKeyPaths)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-playbook failed: %w", err)
	}
	return nil
}

// serverAnsiblePrivateKeyCLIArgs は、鍵候補が1件のときだけ --private-key を使う。
// 複数件ある場合は ansible の --private-key が単一指定しか受け付けないため、
// serverAnsibleCommandEnv() が組み立てる ANSIBLE_SSH_ARGS の IdentityFile 群に委ねる。
func serverAnsiblePrivateKeyCLIArgs(privateKeyPaths []string) []string {
	if len(privateKeyPaths) == 1 {
		return []string{"--private-key", privateKeyPaths[0]}
	}
	return nil
}

func expandServerAnsibleExtraArgs(entries []string) ([]string, error) {
	args := make([]string, 0)
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		tokens, err := splitServerAnsibleExtraArg(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid spec.ansible.extra-args entry %q: %w", trimmed, err)
		}
		if len(tokens) == 0 {
			continue
		}
		if !strings.HasPrefix(tokens[0], "-") {
			tokens[0] = "--" + tokens[0]
		}
		args = append(args, tokens...)
	}
	return args, nil
}

func splitServerAnsibleExtraArg(value string) ([]string, error) {
	tokens := make([]string, 0)
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
				continue
			}
			current.WriteRune(r)
			continue
		}

		if inDouble {
			if r == '"' {
				inDouble = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			current.WriteRune(r)
			continue
		}

		switch {
		case r == '\\':
			escaped = true
		case r == '\'':
			inSingle = true
		case r == '"':
			inDouble = true
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	flush()
	return tokens, nil
}

func serverAnsibleCommandEnv(privateKeyPaths []string) []string {
	env := os.Environ()
	if _, err := os.Stat("ansible.cfg"); err == nil {
		return env
	}
	sshArgs := "-o ControlMaster=auto -o ControlPersist=60s -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes"
	// 鍵候補が複数ある場合、--private-key は使わずここで全候補を IdentityFile として渡す。
	if len(privateKeyPaths) > 1 {
		for _, key := range privateKeyPaths {
			sshArgs += " -i " + key
		}
	}
	return append(env,
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_DEPRECATION_WARNINGS=False",
		"ANSIBLE_REMOTE_TEMP=/tmp",
		"ANSIBLE_SSH_ARGS="+sshArgs,
	)
}
