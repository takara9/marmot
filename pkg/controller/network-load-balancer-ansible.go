package controller

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	networkLoadBalancerAnsiblePlaybookDir    = "/var/lib/marmot/ansible-playbooks"
	networkLoadBalancerAnsibleDefaultUser    = "root"
)

//go:embed gateway-playbooks/network-load-balancer-iptables.yaml.tmpl
var networkLoadBalancerPlaybookTemplate string

var networkLoadBalancerPlaybookDir = networkLoadBalancerAnsiblePlaybookDir
var networkLoadBalancerPrivateKeyPath = marmotd.GatewayPrivateKeyPath()
var runNetworkLoadBalancerPlaybook = runNetworkLoadBalancerPlaybookCommand

type networkLoadBalancerPlaybookData struct {
	RemoteCIDR string                               `json:"remoteCIDR"`
	Listeners  []networkLoadBalancerPlaybookListener `json:"listeners"`
}

type networkLoadBalancerPlaybookListener struct {
	Name         string                           `json:"name"`
	ChainName    string                           `json:"chainName"`
	Protocol     string                           `json:"protocol"`
	VipPort      int                              `json:"vipPort"`
	BackendPort  int                              `json:"backendPort"`
	BackendRules []networkLoadBalancerBackendRule  `json:"backendRules"`
}

type networkLoadBalancerBackendRule struct {
	IP         string `json:"ip"`
	Probability string `json:"probability,omitempty"`
}

type networkLoadBalancerBackendServer struct {
	Name string
	IP   string
}

func desiredNetworkLoadBalancerConfigHash(loadBalancer api.NetworkLoadBalancer, listenerBackends map[string][]networkLoadBalancerBackendServer) (string, error) {
	data, err := buildNetworkLoadBalancerPlaybookData(loadBalancer, listenerBackends)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum), nil
}

func networkLoadBalancerPlaybookPath(loadBalancerID string) string {
	id := strings.TrimSpace(loadBalancerID)
	if id == "" {
		id = "nlb"
	}
	return filepath.Join(networkLoadBalancerPlaybookDir, fmt.Sprintf("network-load-balancer-%s.yaml", id))
}

func buildNetworkLoadBalancerPlaybookData(loadBalancer api.NetworkLoadBalancer, listenerBackends map[string][]networkLoadBalancerBackendServer) (networkLoadBalancerPlaybookData, error) {
	remoteCIDR := strings.TrimSpace(loadBalancer.Spec.RemoteCIDR)
	listeners := make([]networkLoadBalancerPlaybookListener, 0, len(loadBalancer.Spec.Listeners))
	for _, listener := range loadBalancer.Spec.Listeners {
		name := strings.TrimSpace(listener.Name)
		if name == "" {
			continue
		}
		backends := listenerBackends[name]
		rules := buildNetworkLoadBalancerBackendRules(backends)
		listeners = append(listeners, networkLoadBalancerPlaybookListener{
			Name:         name,
			ChainName:    sanitizeNetworkLoadBalancerChainName(loadBalancer.Metadata.Name, name),
			Protocol:     strings.ToLower(strings.TrimSpace(listener.Protocol)),
			VipPort:      listener.VipPort,
			BackendPort:  listener.BackendPort,
			BackendRules: rules,
		})
	}
	return networkLoadBalancerPlaybookData{RemoteCIDR: remoteCIDR, Listeners: listeners}, nil
}

func buildNetworkLoadBalancerBackendRules(backends []networkLoadBalancerBackendServer) []networkLoadBalancerBackendRule {
	rules := make([]networkLoadBalancerBackendRule, 0, len(backends))
	count := len(backends)
	for index, backend := range backends {
		rule := networkLoadBalancerBackendRule{IP: strings.TrimSpace(backend.IP)}
		if index < count-1 && count-index > 1 {
			probability := 1.0 / float64(count-index)
			rule.Probability = strconv.FormatFloat(probability, 'f', 6, 64)
		}
		rules = append(rules, rule)
	}
	return rules
}

func sanitizeNetworkLoadBalancerChainName(loadBalancerName, listenerName string) string {
	base := strings.ToUpper(strings.TrimSpace(loadBalancerName))
	if base == "" {
		base = "NLB"
	}
	listener := strings.ToUpper(strings.TrimSpace(listenerName))
	listener = strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, listener)
	return fmt.Sprintf("MARMOT-NLB-%s-%s", base, listener)
}

func renderNetworkLoadBalancerPlaybook(playbookPath string, data networkLoadBalancerPlaybookData) error {
	if strings.TrimSpace(playbookPath) == "" {
		return fmt.Errorf("playbook path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(playbookPath), 0o755); err != nil {
		return err
	}

	tmpl, err := template.New("network-load-balancer-playbook").Parse(networkLoadBalancerPlaybookTemplate)
	if err != nil {
		return err
	}
	if override, readErr := os.ReadFile(marmotd.NetworkLoadBalancerPlaybookTemplatePath()); readErr == nil && strings.TrimSpace(string(override)) != "" {
		tmpl, err = template.New("network-load-balancer-playbook").Parse(string(override))
		if err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	return os.WriteFile(playbookPath, buf.Bytes(), 0o644)
}

func runNetworkLoadBalancerPlaybookCommand(playbookPath, targetAddress, privateKeyPath string) error {
	address := strings.TrimSpace(targetAddress)
	if address == "" {
		return fmt.Errorf("target address is empty")
	}
	resourceID := resourceIDFromPlaybookPath(playbookPath, "network-load-balancer-")
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
		"-u", networkLoadBalancerAnsibleDefaultUser,
		"--ssh-common-args", "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
	}
	return runAnsiblePlaybookWithLogging(args, "network-load-balancer", resourceID)
}

func networkLoadBalancerBackendAvailabilityMessage(loadBalancer api.NetworkLoadBalancer, listenerBackends map[string][]networkLoadBalancerBackendServer) string {
	missing := make([]string, 0)
	for _, listener := range loadBalancer.Spec.Listeners {
		name := strings.TrimSpace(listener.Name)
		if name == "" {
			continue
		}
		if len(listenerBackends[name]) == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	sort.Strings(missing)
	return fmt.Sprintf("no backend matched for listener(s): %s", strings.Join(missing, ","))
}
