package controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	loadBalancerAnsiblePlaybookDir     = "/var/lib/marmot/ansible-playbooks"
	loadBalancerAnsibleMaxRetryCount   = 5
	loadBalancerAnsibleDefaultUsername = "root"
)

//go:embed gateway-playbooks/load-balancer-haproxy.yaml.tmpl
var loadBalancerPlaybookTemplate string

var (
	loadBalancerPlaybookDir    = loadBalancerAnsiblePlaybookDir
	loadBalancerPrivateKeyPath = marmotd.GatewayPrivateKeyPath()
	runLoadBalancerPlaybook    = runLoadBalancerPlaybookCommand
	readLoadBalancerAgentState = readLoadBalancerAgentStateCommand
)

const loadBalancerAgentStatePath = "/var/lib/marmot/lb-agent/state.json"

type loadBalancerPlaybookData struct {
	TargetIP      string
	HaproxyConfig string
}

type loadBalancerBackendServer struct {
	Name string
	IP   string
}

type loadBalancerAgentState struct {
	LastAppliedHash string `json:"lastAppliedHash"`
	LastAppliedAt   time.Time `json:"lastAppliedAt"`
	LastError       string `json:"lastError,omitempty"`
}

func desiredLoadBalancerConfigHash(loadBalancer api.LoadBalancer, listenerBackends map[string][]loadBalancerBackendServer) string {
	listenerPayloads := make([]string, 0, len(loadBalancer.Spec.Listeners))
	for _, listener := range loadBalancer.Spec.Listeners {
		labelPairs := make([]string, 0, len(listener.BackendSelector.MatchLabels))
		for key, value := range listener.BackendSelector.MatchLabels {
			labelPairs = append(labelPairs, strings.TrimSpace(key)+"="+strings.TrimSpace(value))
		}
		sort.Strings(labelPairs)
		backendPairs := make([]string, 0)
		if backends, ok := listenerBackends[strings.TrimSpace(listener.Name)]; ok {
			backendPairs = make([]string, 0, len(backends))
			for _, backend := range backends {
				backendPairs = append(backendPairs, strings.TrimSpace(backend.Name)+"@"+strings.TrimSpace(backend.IP))
			}
			sort.Strings(backendPairs)
		}
		listenerPayloads = append(listenerPayloads, strings.Join([]string{
			strings.TrimSpace(listener.Name),
			strings.ToUpper(strings.TrimSpace(listener.Protocol)),
			fmt.Sprintf("%d", listener.VipPort),
			fmt.Sprintf("%d", listener.BackendPort),
			strings.ToLower(strings.TrimSpace(listener.LoadBalancingAlgorithm)),
			strings.Join(labelPairs, ","),
			strings.Join(backendPairs, ","),
		}, "|"))
	}
	sort.Strings(listenerPayloads)
	payload := strings.Join([]string{
		strings.TrimSpace(api.LoadBalancerID(loadBalancer)),
		strings.TrimSpace(loadBalancer.Spec.BindPublicIpAddress),
		strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork),
		strings.TrimSpace(loadBalancer.Spec.RemoteCIDR),
		strings.Join(listenerPayloads, "||"),
	}, "###")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

func renderLoadBalancerPlaybook(playbookPath string, targetIP string, loadBalancer api.LoadBalancer, listenerBackends map[string][]loadBalancerBackendServer) error {
	if strings.TrimSpace(playbookPath) == "" {
		return fmt.Errorf("playbook path is empty")
	}
	if strings.TrimSpace(targetIP) == "" {
		return fmt.Errorf("target ip is empty")
	}
	if err := os.MkdirAll(filepath.Dir(playbookPath), 0o755); err != nil {
		return err
	}

	haproxyCfg, err := buildLoadBalancerHAProxyConfig(loadBalancer, listenerBackends)
	if err != nil {
		return err
	}
	haproxyCfg = indentMultiline(haproxyCfg, 10)

	tmpl, err := template.New("load-balancer-playbook").Parse(loadBalancerPlaybookTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, loadBalancerPlaybookData{TargetIP: strings.TrimSpace(targetIP), HaproxyConfig: haproxyCfg}); err != nil {
		return err
	}
	return os.WriteFile(playbookPath, buf.Bytes(), 0o644)
}

func indentMultiline(input string, spaces int) string {
	if spaces <= 0 {
		return input
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(input, "\n"), "\n")
	for i := range lines {
		if lines[i] == "" {
			lines[i] = prefix
			continue
		}
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}

func runLoadBalancerPlaybookCommand(playbookPath, targetAddress, privateKeyPath string) error {
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
		"-u", loadBalancerAnsibleDefaultUsername,
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

func readLoadBalancerAgentStateCommand(targetAddress, privateKeyPath string) (loadBalancerAgentState, error) {
	address := strings.TrimSpace(targetAddress)
	if address == "" {
		return loadBalancerAgentState{}, fmt.Errorf("target address is empty")
	}
	key := strings.TrimSpace(privateKeyPath)
	if key == "" {
		return loadBalancerAgentState{}, fmt.Errorf("private key path is empty")
	}
	if _, err := os.Stat(key); err != nil {
		return loadBalancerAgentState{}, fmt.Errorf("private key is not available: %w", err)
	}

	args := []string{
		"-i", key,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		loadBalancerAnsibleDefaultUsername + "@" + address,
		"cat", loadBalancerAgentStatePath,
	}
	cmd := exec.Command("ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return loadBalancerAgentState{}, fmt.Errorf("ssh state read failed: %w", err)
		}
		return loadBalancerAgentState{}, fmt.Errorf("ssh state read failed: %w: %s", err, trimmed)
	}

	var state loadBalancerAgentState
	if err := json.Unmarshal(output, &state); err != nil {
		return loadBalancerAgentState{}, fmt.Errorf("failed to decode lb agent state: %w", err)
	}
	return state, nil
}

func buildLoadBalancerHAProxyConfig(loadBalancer api.LoadBalancer, listenerBackends map[string][]loadBalancerBackendServer) (string, error) {
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