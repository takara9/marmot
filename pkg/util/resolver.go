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
// If /etc/resolv.conf already has a nameserver entry that matches the IP derived from dnsListenAddr,
// it skips the rewrite entirely.
func SetupLocalResolver(dnsListenAddr string) error {
	nameserver := nameserverForDNSListenAddr(dnsListenAddr)

	if current, err := currentNameserverInResolvConf(); err == nil && current == nameserver {
		slog.Info("resolv.conf nameserver already matches dns_listen_addr; skipping rewrite", "nameserver", nameserver)
		return nil
	}

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

// currentNameserverInResolvConf returns the first nameserver IP found in /etc/resolv.conf.
func currentNameserverInResolvConf() (string, error) {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "", err
	}
	return parseNameserverFromResolvConf(string(data))
}

// parseNameserverFromResolvConf extracts the first nameserver IP from resolv.conf content.
func parseNameserverFromResolvConf(content string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}
	return "", fmt.Errorf("no nameserver entry found in /etc/resolv.conf")
}

// NameserverForDNSListenAddr extracts the nameserver IP from a dns_listen_addr string (host:port).
// "0.0.0.0" and "" are normalized to "127.0.0.1".
func NameserverForDNSListenAddr(dnsListenAddr string) string {
	return nameserverForDNSListenAddr(dnsListenAddr)
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
