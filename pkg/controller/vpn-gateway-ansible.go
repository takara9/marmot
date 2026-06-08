package controller

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	vpnGatewayAnsiblePlaybookDir     = "/var/lib/marmot/ansible-playbooks"
	vpnGatewayAnsibleMaxRetryCount   = 5
	vpnGatewayAnsibleDefaultUsername = "root"
)

//go:embed gateway-playbooks/vpn-gateway-openvpn.yaml.tmpl
var vpnGatewayPlaybookTemplate string

var (
	vpnGatewayPlaybookDir    = vpnGatewayAnsiblePlaybookDir
	vpnGatewayPrivateKeyPath = marmotd.GatewayPrivateKeyPath()
	runVpnGatewayPlaybook    = runVpnGatewayPlaybookCommand
)

type vpnGatewayPlaybookData struct {
	TargetIP             string
	PublicIP             string
	InternalNetworkCIDR  string
	InternalNetworkAddr  string
	InternalNetworkMask  string
	InternalNetworkName  string
	RemoteCIDRs          []string
	ClientProfilePath    string
	ClientProfileNetwork string
}

func desiredVpnGatewayConfigHash(vpnGateway api.VpnGateway) string {
	remoteCIDRs := make([]string, 0)
	if vpnGateway.Spec.RemoteCIDRs != nil {
		remoteCIDRs = make([]string, 0, len(*vpnGateway.Spec.RemoteCIDRs))
		for _, cidr := range *vpnGateway.Spec.RemoteCIDRs {
			trimmed := strings.TrimSpace(cidr)
			if trimmed != "" {
				remoteCIDRs = append(remoteCIDRs, trimmed)
			}
		}
	}
	sort.Strings(remoteCIDRs)
	payload := strings.Join([]string{
		strings.TrimSpace(api.VpnGatewayID(vpnGateway)),
		strings.TrimSpace(vpnGateway.Spec.BindPublicIpAddress),
		strings.TrimSpace(vpnGateway.Spec.InternalVirtualNetwork),
		strings.Join(remoteCIDRs, ","),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

func renderVpnGatewayPlaybook(playbookPath string, targetIP string, vpnGateway api.VpnGateway, internalCIDR string) error {
	if strings.TrimSpace(playbookPath) == "" {
		return fmt.Errorf("playbook path is empty")
	}
	if strings.TrimSpace(targetIP) == "" {
		return fmt.Errorf("target ip is empty")
	}
	if strings.TrimSpace(internalCIDR) == "" {
		return fmt.Errorf("internal cidr is empty")
	}
	networkAddr, networkMask, err := networkAndMaskFromCIDR(internalCIDR)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(playbookPath), 0o755); err != nil {
		return err
	}

	tmpl, err := template.New("vpn-gateway-playbook").Parse(vpnGatewayPlaybookTemplate)
	if err != nil {
		return err
	}
	if override, readErr := os.ReadFile(marmotd.GatewayPlaybookTemplatePath()); readErr == nil && strings.TrimSpace(string(override)) != "" {
		tmpl, err = template.New("vpn-gateway-playbook").Parse(string(override))
		if err != nil {
			return err
		}
	}

	remoteCIDRs := normalizeVpnGatewayRemoteCIDRs(vpnGateway.Spec.RemoteCIDRs)
	profileNetwork := strings.TrimSpace(vpnGateway.Spec.InternalVirtualNetwork)
	profilePath := filepath.Join("/var/lib/marmot/vpn", filepath.Base(profileNetwork)+".ovpn")

	data := vpnGatewayPlaybookData{
		TargetIP:             strings.TrimSpace(targetIP),
		PublicIP:             strings.TrimSpace(vpnGateway.Spec.BindPublicIpAddress),
		InternalNetworkCIDR:  strings.TrimSpace(internalCIDR),
		InternalNetworkAddr:  networkAddr,
		InternalNetworkMask:  networkMask,
		InternalNetworkName:  profileNetwork,
		RemoteCIDRs:          remoteCIDRs,
		ClientProfilePath:    profilePath,
		ClientProfileNetwork: filepath.Base(profileNetwork),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	return os.WriteFile(playbookPath, buf.Bytes(), 0o644)
}

func normalizeVpnGatewayRemoteCIDRs(values *[]string) []string {
	result := make([]string, 0)
	if values != nil {
		result = make([]string, 0, len(*values))
		for _, cidr := range *values {
			trimmed := strings.TrimSpace(cidr)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
	}
	if len(result) == 0 {
		return []string{"0.0.0.0/0"}
	}
	sort.Strings(result)
	return result
}

func networkAndMaskFromCIDR(cidr string) (string, string, error) {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return "", "", fmt.Errorf("invalid cidr %q: %w", cidr, err)
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return "", "", fmt.Errorf("cidr %q is not an IPv4 network", cidr)
	}
	mask := net.IP(ipNet.Mask).To4()
	if mask == nil {
		return "", "", fmt.Errorf("cidr %q has invalid IPv4 mask", cidr)
	}
	return ip4.String(), mask.String(), nil
}

func runVpnGatewayPlaybookCommand(playbookPath, gatewayAddress, privateKeyPath string) error {
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
		"-u", vpnGatewayAnsibleDefaultUsername,
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
