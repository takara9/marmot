package controller

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	applicationLoadBalancerAnsiblePlaybookDir     = "/var/lib/marmot/ansible-playbooks"
	applicationLoadBalancerAnsibleMaxRetryCount   = 5
	applicationLoadBalancerAnsibleDefaultUsername = "root"
	applicationLoadBalancerDesiredConfigDir       = "/var/lib/marmot/ansible-playbooks/desire-configs"
)

//go:embed gateway-playbooks/load-balancer-haproxy.yaml.tmpl
var applicationLoadBalancerPlaybookTemplate string

var (
	applicationLoadBalancerPlaybookDir           = applicationLoadBalancerAnsiblePlaybookDir
	applicationLoadBalancerDesiredDir            = applicationLoadBalancerDesiredConfigDir
	applicationLoadBalancerPrivateKeyPath        = marmotd.GatewayPrivateKeyPath()
	runApplicationLoadBalancerPlaybook           = runApplicationLoadBalancerPlaybookCommand
	readApplicationLoadBalancerAgentState        = readApplicationLoadBalancerAgentStateCommand
	readApplicationLoadBalancerDesiredConfigHash = readApplicationLoadBalancerDesiredConfigHashCommand
)

const applicationLoadBalancerAgentStatePath = "/var/lib/marmot/lb-agent/state.json"

type applicationLoadBalancerPlaybookData struct {
	TargetIP                string
	DesiredConfigSourcePath string
	DesiredConfigPath       string
}

type applicationLoadBalancerBackendServer struct {
	Name string
	IP   string
}

type applicationLoadBalancerAgentState struct {
	LastAppliedHash string    `json:"lastAppliedHash"`
	LastAppliedAt   time.Time `json:"lastAppliedAt"`
	LastError       string    `json:"lastError,omitempty"`
}

func desiredApplicationLoadBalancerConfigHash(loadBalancer api.ApplicationLoadBalancer, listenerBackends map[string][]applicationLoadBalancerBackendServer) (string, error) {
	haproxyCfg, err := buildApplicationLoadBalancerHAProxyConfig(loadBalancer, listenerBackends)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(haproxyCfg))
	return fmt.Sprintf("%x", sum), nil
}

func applicationLoadBalancerDesiredConfigPath(loadBalancerID string) string {
	id := strings.TrimSpace(loadBalancerID)
	if id == "" {
		id = "lb"
	}
	return filepath.Join(applicationLoadBalancerDesiredDir, fmt.Sprintf("haproxy-desired-%s.cfg", id))
}

func writeApplicationLoadBalancerDesiredConfig(desiredConfigPath string, loadBalancer api.ApplicationLoadBalancer, listenerBackends map[string][]applicationLoadBalancerBackendServer) (string, error) {
	if strings.TrimSpace(desiredConfigPath) == "" {
		return "", fmt.Errorf("desired config path is empty")
	}
	haproxyCfg, err := buildApplicationLoadBalancerHAProxyConfig(loadBalancer, listenerBackends)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(desiredConfigPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(desiredConfigPath, []byte(haproxyCfg), 0o644); err != nil {
		return "", err
	}
	return fileSHA256Hex(desiredConfigPath)
}

func fileSHA256Hex(path string) (string, error) {
	content, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum), nil
}

func renderApplicationLoadBalancerPlaybook(playbookPath string, targetIP string, desiredConfigSourcePath string, desiredConfigPath string) error {
	if strings.TrimSpace(playbookPath) == "" {
		return fmt.Errorf("playbook path is empty")
	}
	if strings.TrimSpace(targetIP) == "" {
		return fmt.Errorf("target ip is empty")
	}
	if strings.TrimSpace(desiredConfigSourcePath) == "" {
		return fmt.Errorf("desired config source path is empty")
	}
	if strings.TrimSpace(desiredConfigPath) == "" {
		return fmt.Errorf("desired config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(playbookPath), 0o755); err != nil {
		return err
	}

	tmpl, err := template.New("load-balancer-playbook").Parse(applicationLoadBalancerPlaybookTemplate)
	if err != nil {
		return err
	}
	if override, readErr := os.ReadFile(marmotd.LoadBalancerPlaybookTemplatePath()); readErr == nil && strings.TrimSpace(string(override)) != "" {
		tmpl, err = template.New("load-balancer-playbook").Parse(string(override))
		if err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	data := applicationLoadBalancerPlaybookData{
		TargetIP:                strings.TrimSpace(targetIP),
		DesiredConfigSourcePath: strings.TrimSpace(desiredConfigSourcePath),
		DesiredConfigPath:       strings.TrimSpace(desiredConfigPath),
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	return os.WriteFile(playbookPath, buf.Bytes(), 0o644)
}

func runApplicationLoadBalancerPlaybookCommand(playbookPath, targetAddress, privateKeyPath string) error {
	address := strings.TrimSpace(targetAddress)
	if address == "" {
		return fmt.Errorf("target address is empty")
	}
	key := strings.TrimSpace(privateKeyPath)
	if key == "" {
		return fmt.Errorf("private key path is empty")
	}
	if _, err := os.Stat(key); err != nil {
		return fmt.Errorf("private key is not available: %w", err)
	}

	args := []string{
		"-i", address + ",",
		playbookPath,
		"--private-key", key,
		"-u", applicationLoadBalancerAnsibleDefaultUsername,
		"--ssh-common-args", "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
	}
	cmd := exec.Command("ansible-playbook", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("ansible-playbook failed: %w", err)
		}
		return fmt.Errorf("ansible-playbook failed: %w: %s", err, trimmed)
	}
	return nil
}

func readApplicationLoadBalancerAgentStateCommand(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
	address := strings.TrimSpace(targetAddress)
	if address == "" {
		return applicationLoadBalancerAgentState{}, fmt.Errorf("target address is empty")
	}
	key := strings.TrimSpace(privateKeyPath)
	if key == "" {
		return applicationLoadBalancerAgentState{}, fmt.Errorf("private key path is empty")
	}
	if _, err := os.Stat(key); err != nil {
		return applicationLoadBalancerAgentState{}, fmt.Errorf("private key is not available: %w", err)
	}

	args := []string{
		"-i", key,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		applicationLoadBalancerAnsibleDefaultUsername + "@" + address,
		"cat", applicationLoadBalancerAgentStatePath,
	}
	cmd := exec.Command("ssh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		trimmed := strings.TrimSpace(stderr.String())
		if trimmed == "" {
			return applicationLoadBalancerAgentState{}, fmt.Errorf("ssh state read failed: %w", err)
		}
		return applicationLoadBalancerAgentState{}, fmt.Errorf("ssh state read failed: %w: %s", err, trimmed)
	}

	state, err := decodeApplicationLoadBalancerAgentState(output)
	if err != nil {
		return applicationLoadBalancerAgentState{}, fmt.Errorf("failed to decode lb agent state: %w", err)
	}
	return state, nil
}

func decodeApplicationLoadBalancerAgentState(output []byte) (applicationLoadBalancerAgentState, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return applicationLoadBalancerAgentState{}, fmt.Errorf("empty payload")
	}

	var state applicationLoadBalancerAgentState
	if err := json.Unmarshal(trimmed, &state); err != nil {
		return applicationLoadBalancerAgentState{}, err
	}
	return state, nil
}

func readApplicationLoadBalancerDesiredConfigHashCommand(targetAddress, privateKeyPath, desiredConfigPath string) (string, error) {
	address := strings.TrimSpace(targetAddress)
	if address == "" {
		return "", fmt.Errorf("target address is empty")
	}
	key := strings.TrimSpace(privateKeyPath)
	if key == "" {
		return "", fmt.Errorf("private key path is empty")
	}
	if _, err := os.Stat(key); err != nil {
		return "", fmt.Errorf("private key is not available: %w", err)
	}
	path := strings.TrimSpace(desiredConfigPath)
	if path == "" {
		return "", fmt.Errorf("desired config path is empty")
	}

	args := []string{
		"-i", key,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		applicationLoadBalancerAnsibleDefaultUsername + "@" + address,
		"sha256sum", path,
	}
	cmd := exec.Command("ssh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		trimmed := strings.TrimSpace(stderr.String())
		if trimmed == "" {
			return "", fmt.Errorf("ssh desired config hash read failed: %w", err)
		}
		return "", fmt.Errorf("ssh desired config hash read failed: %w: %s", err, trimmed)
	}
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 1 {
		return "", fmt.Errorf("failed to parse sha256sum output")
	}
	return strings.TrimSpace(fields[0]), nil
}

func buildApplicationLoadBalancerHAProxyConfig(loadBalancer api.ApplicationLoadBalancer, listenerBackends map[string][]applicationLoadBalancerBackendServer) (string, error) {
	id := strings.TrimSpace(api.LoadBalancerID(loadBalancer))
	if id == "" {
		id = strings.TrimSpace(loadBalancer.Metadata.Name)
	}
	if id == "" {
		id = "lb"
	}

	var b strings.Builder
	b.WriteString("global\n")
	b.WriteString("  daemon\n")
	b.WriteString("  maxconn 2048\n\n")
	b.WriteString("defaults\n")
	b.WriteString("  timeout connect 5s\n")
	b.WriteString("  timeout client 60s\n")
	b.WriteString("  timeout server 60s\n\n")

	for index, listener := range loadBalancer.Spec.Listeners {
		protocol := strings.ToUpper(strings.TrimSpace(listener.Protocol))
		mode := "tcp"
		if protocol == "HTTP" {
			mode = "http"
		}
		algorithm := strings.ToLower(strings.TrimSpace(listener.LoadBalancingAlgorithm))
		if algorithm == "" {
			algorithm = "roundrobin"
		}
		name := strings.TrimSpace(listener.Name)
		if name == "" {
			name = fmt.Sprintf("listener-%d", index+1)
		}
		frontendName := sanitizeHAProxyToken(fmt.Sprintf("%s-fe-%s", id, name))
		backendName := sanitizeHAProxyToken(fmt.Sprintf("%s-be-%s", id, name))

		b.WriteString("frontend " + frontendName + "\n")
		b.WriteString(fmt.Sprintf("  bind %s:%d\n", strings.TrimSpace(loadBalancer.Spec.BindPublicIpAddress), listener.VipPort))
		b.WriteString("  mode " + mode + "\n")
		b.WriteString("  default_backend " + backendName + "\n\n")

		b.WriteString("backend " + backendName + "\n")
		b.WriteString("  mode " + mode + "\n")
		b.WriteString("  balance " + algorithm + "\n")
		if listener.HealthCheck != nil && listener.HealthCheck.Enabled && mode == "http" {
			path := strings.TrimSpace(listener.HealthCheck.Path)
			if path == "" {
				path = "/healthz"
			}
			b.WriteString("  option httpchk GET " + path + "\n")
		}
		if listener.SessionPersistence != nil && listener.SessionPersistence.Enabled && mode == "http" {
			cookieName := strings.TrimSpace(listener.SessionPersistence.CookieName)
			if cookieName == "" {
				cookieName = sanitizeCookieName(loadBalancer.Metadata.Name)
			}
			b.WriteString("  cookie " + cookieName + " insert indirect nocache\n")
		}
		backends := listenerBackends[strings.TrimSpace(listener.Name)]
		if len(backends) == 0 {
			b.WriteString(fmt.Sprintf("  server-template srv 20 _placeholder_:%d check disabled\n\n", listener.BackendPort))
			continue
		}
		for backendIndex, backend := range backends {
			backendIP := strings.TrimSpace(backend.IP)
			if backendIP == "" {
				continue
			}
			serverName := sanitizeHAProxyToken(fmt.Sprintf("srv-%d-%s", backendIndex+1, backend.Name))
			if serverName == "" {
				serverName = fmt.Sprintf("srv-%d", backendIndex+1)
			}
			line := fmt.Sprintf("  server %s %s:%d check", serverName, backendIP, listener.BackendPort)
			if listener.SessionPersistence != nil && listener.SessionPersistence.Enabled && mode == "http" {
				line += fmt.Sprintf(" cookie %s", serverName)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

func sanitizeHAProxyToken(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return "item"
	}
	var b strings.Builder
	for i, r := range input {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlpha || r == '-' || r == '_' {
			if i == 0 && (r >= '0' && r <= '9') {
				b.WriteByte('a')
			}
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	result := strings.Trim(b.String(), "-_")
	if result == "" {
		return "item"
	}
	if result[0] >= '0' && result[0] <= '9' {
		return "a" + result
	}
	return result
}

func sanitizeCookieName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "MARMOTLB"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" {
		out = "MARMOTLB"
	}
	first := out[0]
	if (first < 'A' || first > 'Z') && (first < 'a' || first > 'z') {
		out = "L" + out
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
