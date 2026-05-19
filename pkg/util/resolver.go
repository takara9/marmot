package util

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
)

const marmotResolvConfTemplate = "# generate by marmotd\nnameserver %s\noptions edns0 trust-ad\nsearch host-bridge\n"

// SetupLocalResolver disables systemd-resolved and rewrites /etc/resolv.conf for marmot internal DNS.
func SetupLocalResolver(dnsListenAddr string) error {
	nameserver := nameserverForDNSListenAddr(dnsListenAddr)
	resolvConfContent := fmt.Sprintf(marmotResolvConfTemplate, nameserver)

	if err := runSystemctlResolved("stop"); err != nil {
		return err
	}
	if err := runSystemctlResolved("disable"); err != nil {
		return err
	}
	if err := os.Remove("/etc/resolv.conf"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove /etc/resolv.conf: %w", err)
	}
	if err := os.WriteFile("/etc/resolv.conf", []byte(resolvConfContent), 0644); err != nil {
		return fmt.Errorf("write /etc/resolv.conf: %w", err)
	}
	return nil
}

func nameserverForDNSListenAddr(dnsListenAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(dnsListenAddr))
	if err != nil {
		return "127.0.0.1"
	}

	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	switch host {
	case "", "0.0.0.0":
		return "127.0.0.1"
	case "::":
		return "::1"
	default:
		return host
	}
}

func runSystemctlResolved(action string) error {
	cmd := exec.Command("systemctl", action, "systemd-resolved.service")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	msg := string(output)
	if strings.Contains(msg, "Unit systemd-resolved.service could not be found") ||
		strings.Contains(msg, "Loaded: not-found") {
		slog.Warn("systemd-resolved.service is not installed; continue", "action", action)
		return nil
	}

	return fmt.Errorf("%s systemd-resolved.service: %w: %s", action, err, strings.TrimSpace(msg))
}
