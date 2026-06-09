package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	if server.Spec.AnsiblePlaybook == nil || strings.TrimSpace(*server.Spec.AnsiblePlaybook) == "" {
		return nil
	}

	targetAddress, err := validateServerAnsiblePlaybookSpec(server)
	if err != nil {
		return err
	}

	serverID, err := extractSuccessID(createResponse)
	if err != nil {
		return fmt.Errorf("failed to parse create response id for ansible apply: %w", err)
	}

	fmt.Fprintln(os.Stderr, "OS起動待機中.....")
	if err := waitServerRunning(m, serverID, serverAnsibleWaitTimeout, serverAnsibleWaitPollInterval); err != nil {
		return err
	}

	playbookPath, err := resolveServerAnsiblePlaybookPath(*server.Spec.AnsiblePlaybook)
	if err != nil {
		return err
	}
	privateKeyPath, err := resolveServerAnsiblePrivateKeyPath()
	if err != nil {
		return err
	}

	if err := waitServerAnsiblePingReady(targetAddress, privateKeyPath, serverAnsiblePingTimeout, serverAnsiblePingPollInterval); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "playbook 適用開始.....")
	if err := runServerAnsiblePlaybook(playbookPath, targetAddress, privateKeyPath); err != nil {
		return err
	}
	return nil
}

func validateServerAnsiblePlaybookSpec(server api.Server) (string, error) {
	if server.Spec.AnsiblePlaybook == nil || strings.TrimSpace(*server.Spec.AnsiblePlaybook) == "" {
		return "", nil
	}
	if server.Spec.NetworkInterface == nil || len(*server.Spec.NetworkInterface) == 0 {
		return "", fmt.Errorf("spec.ansible-playbook requires spec.networkInterface with host-bridge and static address")
	}
	for _, nic := range *server.Spec.NetworkInterface {
		if strings.TrimSpace(nic.Networkname) != "host-bridge" {
			continue
		}
		if nic.Address == nil || strings.TrimSpace(*nic.Address) == "" {
			return "", fmt.Errorf("spec.ansible-playbook requires host-bridge address to be set")
		}
		return strings.TrimSpace(*nic.Address), nil
	}
	return "", fmt.Errorf("spec.ansible-playbook can be used only when host-bridge is specified in spec.networkInterface")
}

func extractSuccessID(body []byte) (string, error) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	id := strings.TrimSpace(fmt.Sprint(data["id"]))
	if id == "" || id == "<nil>" {
		return "", fmt.Errorf("id is empty")
	}
	return id, nil
}

func waitServerRunning(m *client.MarmotEndpoint, serverID string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		body, _, err := m.GetServerById(serverID)
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
			return fmt.Errorf("timeout waiting for server %s to become RUNNING", serverID)
		}
		time.Sleep(interval)
	}
}

func resolveServerAnsiblePlaybookPath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("spec.ansible-playbook is empty")
	}
	if !filepath.IsAbs(p) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		p = filepath.Join(cwd, p)
	}
	if info, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("ansible playbook file is not found: %s", p)
	} else if info.IsDir() {
		return "", fmt.Errorf("ansible playbook path is a directory: %s", p)
	}
	return p, nil
}

func resolveServerAnsiblePrivateKeyPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(serverAnsiblePrivateKeyEnvName)); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s points to missing key: %s", serverAnsiblePrivateKeyEnvName, p)
		}
		return p, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no private key found; set %s or place key under ~/.ssh", serverAnsiblePrivateKeyEnvName)
}

func waitServerAnsiblePingReady(targetAddress, privateKeyPath string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := runServerAnsiblePing(targetAddress, privateKeyPath); err == nil {
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

func runServerAnsiblePing(targetAddress, privateKeyPath string) error {
	args := []string{
		"all",
		"-i", targetAddress + ",",
		"-m", "ping",
		"-u", serverAnsibleDefaultUser,
		"--private-key", privateKeyPath,
	}
	cmd := serverAnsibleExecCommand("ansible", args...)
	cmd.Env = serverAnsibleCommandEnv()
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

func runServerAnsiblePlaybook(playbookPath, targetAddress, privateKeyPath string) error {
	args := []string{
		"-i", targetAddress + ",",
		playbookPath,
		"--private-key", privateKeyPath,
		"-u", serverAnsibleDefaultUser,
	}
	cmd := serverAnsibleExecCommand("ansible-playbook", args...)
	cmd.Env = serverAnsibleCommandEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-playbook failed: %w", err)
	}
	return nil
}

func serverAnsibleCommandEnv() []string {
	env := os.Environ()
	if _, err := os.Stat("ansible.cfg"); err == nil {
		return env
	}
	return append(env,
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_DEPRECATION_WARNINGS=False",
		"ANSIBLE_SSH_ARGS=-o ControlMaster=auto -o ControlPersist=60s -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes",
	)
}
