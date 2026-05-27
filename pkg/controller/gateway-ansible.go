package controller

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	gatewayAnsiblePlaybookDir     = "/var/lib/marmot/ansible-playbooks"
	gatewayAnsibleMaxRetryCount   = 3
	gatewayAnsibleDefaultUsername = "root"
)

//go:embed gateway-playbooks/gateway-iptables.yaml.tmpl
var gatewayPlaybookTemplate string

var (
	gatewayPlaybookDir    = gatewayAnsiblePlaybookDir
	gatewayPrivateKeyPath = marmotd.GatewayPrivateKeyPath()
	runGatewayPlaybook    = runGatewayPlaybookCommand
)

type gatewayPortRule struct {
	Protocol string
	Port     int
}

type gatewayPlaybookData struct {
	TargetIP string
	RemoteCIDR string
	Ports    []gatewayPortRule
}

func desiredGatewayConfigHash(gateway api.Gateway, targetIP string) string {
	ports := append([]string(nil), gateway.Spec.ServerPorts...)
	for i := range ports {
		ports[i] = strings.TrimSpace(ports[i])
	}
	sort.Strings(ports)
	payload := strings.Join([]string{
		strings.TrimSpace(api.GatewayID(gateway)),
		strings.TrimSpace(gateway.Spec.BindPublicIpAddress),
		strings.TrimSpace(gateway.Spec.InternalServerName),
		strings.TrimSpace(gateway.Spec.InternalVirtualNetwork),
		gatewayRemoteCIDR(gateway.Spec),
		strings.TrimSpace(targetIP),
		strings.Join(ports, ","),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

func renderGatewayPlaybook(playbookPath string, targetIP string, serverPorts []string, remoteCIDR string) error {
	if strings.TrimSpace(playbookPath) == "" {
		return fmt.Errorf("playbook path is empty")
	}
	if strings.TrimSpace(targetIP) == "" {
		return fmt.Errorf("target ip is empty")
	}
	if strings.TrimSpace(remoteCIDR) == "" {
		return fmt.Errorf("remote CIDR is empty")
	}
	portRules, err := buildGatewayPortRules(serverPorts)
	if err != nil {
		return err
	}
	if len(portRules) == 0 {
		return fmt.Errorf("no server ports are defined")
	}
	if err := os.MkdirAll(filepath.Dir(playbookPath), 0o755); err != nil {
		return err
	}

	tmpl, err := template.New("gateway-playbook").Parse(gatewayPlaybookTemplate)
	if err != nil {
		return err
	}
	if override, readErr := os.ReadFile(marmotd.GatewayPlaybookTemplatePath()); readErr == nil && strings.TrimSpace(string(override)) != "" {
		tmpl, err = template.New("gateway-playbook").Parse(string(override))
		if err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, gatewayPlaybookData{TargetIP: strings.TrimSpace(targetIP), RemoteCIDR: strings.TrimSpace(remoteCIDR), Ports: portRules}); err != nil {
		return err
	}
	return os.WriteFile(playbookPath, buf.Bytes(), 0o644)
}

func gatewayRemoteCIDR(spec api.GatewaySpec) string {
	remoteCIDR := strings.TrimSpace(spec.RemoteCIDR)
	if remoteCIDR == "" {
		return "0.0.0.0/0"
	}
	return remoteCIDR
}

func buildGatewayPortRules(serverPorts []string) ([]gatewayPortRule, error) {
	rules := make([]gatewayPortRule, 0, len(serverPorts))
	for _, spec := range serverPorts {
		trimmed := strings.TrimSpace(spec)
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid port spec: %s", trimmed)
		}
		port, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid port spec: %s", trimmed)
		}
		protocol := strings.ToLower(strings.TrimSpace(parts[1]))
		if protocol != "tcp" && protocol != "udp" {
			return nil, fmt.Errorf("invalid protocol in port spec: %s", trimmed)
		}
		rules = append(rules, gatewayPortRule{Protocol: protocol, Port: port})
	}
	return rules, nil
}

func runGatewayPlaybookCommand(playbookPath, gatewayAddress, privateKeyPath string) error {
	address := strings.TrimSpace(gatewayAddress)
	if address == "" {
		return fmt.Errorf("gateway address is empty")
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
		"-u", gatewayAnsibleDefaultUsername,
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
