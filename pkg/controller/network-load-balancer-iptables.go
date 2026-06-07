package controller

import (
	"bytes"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"

	"github.com/takara9/marmot/api"
)

var runNetworkLoadBalancerApply = runNetworkLoadBalancerApplyCommand
var runNetworkLoadBalancerCleanup = runNetworkLoadBalancerCleanupCommand

func runNetworkLoadBalancerApplyCommand(script string) error {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return fmt.Errorf("iptables restore script is empty")
	}

	cmd := exec.Command("iptables-restore")
	cmd.Stdin = strings.NewReader(trimmed + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("iptables-restore failed: %w", err)
		}
		return fmt.Errorf("iptables-restore failed: %w: %s", err, msg)
	}
	return nil
}

func runNetworkLoadBalancerCleanupCommand(spec api.NetworkLoadBalancerSpec, chainPrefix string) error {
	listenerChains, publicIP, remoteCIDR, err := buildNetworkLoadBalancerCleanupContext(spec, chainPrefix)
	if err != nil {
		return err
	}

	for _, item := range listenerChains {
		args := []string{
			"-t", "nat",
			"-D", "PREROUTING",
			"-d", publicIP,
			"-s", remoteCIDR,
			"-p", item.protocol,
			"--dport", strconv.Itoa(item.vipPort),
			"-m", "conntrack",
			"--ctstate", "NEW",
			"-j", item.chain,
		}
		_ = exec.Command("iptables", args...).Run()

		_ = exec.Command("iptables", "-t", "nat", "-F", item.chain).Run()
		_ = exec.Command("iptables", "-t", "nat", "-X", item.chain).Run()
	}

	return nil
}

type networkLoadBalancerCleanupListener struct {
	chain    string
	protocol string
	vipPort  int
}

func buildNetworkLoadBalancerCleanupContext(spec api.NetworkLoadBalancerSpec, chainPrefix string) ([]networkLoadBalancerCleanupListener, string, string, error) {
	publicIP, err := normalizeNetworkLoadBalancerIPv4(spec.BindPublicIpAddress)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid bind public ip: %w", err)
	}
	remoteCIDR, err := normalizeNetworkLoadBalancerCIDR(spec.RemoteCIDR)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid remote cidr: %w", err)
	}

	prefix := strings.TrimSpace(chainPrefix)
	if prefix == "" {
		prefix = "NLB"
	}

	out := make([]networkLoadBalancerCleanupListener, 0, len(spec.Listeners))
	idx := 0
	for _, listener := range spec.Listeners {
		name := strings.TrimSpace(listener.Name)
		if name == "" {
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(listener.Protocol))
		if protocol != "tcp" && protocol != "udp" {
			return nil, "", "", fmt.Errorf("listener %q protocol must be tcp or udp", name)
		}
		if listener.VipPort < 1 || listener.VipPort > 65535 {
			return nil, "", "", fmt.Errorf("listener %q vipPort must be 1-65535", name)
		}
		out = append(out, networkLoadBalancerCleanupListener{
			chain:    fmt.Sprintf("%s_%d", prefix, idx),
			protocol: protocol,
			vipPort:  listener.VipPort,
		})
		idx++
	}

	return out, publicIP, remoteCIDR, nil
}

func normalizeNetworkLoadBalancerIPv4(raw string) (string, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if !addr.Is4() {
		return "", fmt.Errorf("only IPv4 is supported in initial implementation")
	}
	return addr.String(), nil
}

func normalizeNetworkLoadBalancerCIDR(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "0.0.0.0/0", nil
	}
	prefix, err := netip.ParsePrefix(trimmed)
	if err != nil {
		return "", err
	}
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("only IPv4 is supported in initial implementation")
	}
	return prefix.String(), nil
}
