package util

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const marmotResolvConfContent = "# generate by marmotd\nnameserver 127.0.0.1\noptions edns0 trust-ad\nsearch  host-bridge\n"

// SetupLocalResolver disables systemd-resolved and rewrites /etc/resolv.conf for marmot internal DNS.
func SetupLocalResolver() error {
	if err := runSystemctlResolved("stop"); err != nil {
		return err
	}
	if err := runSystemctlResolved("disable"); err != nil {
		return err
	}
	if err := os.Remove("/etc/resolv.conf"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove /etc/resolv.conf: %w", err)
	}
	if err := os.WriteFile("/etc/resolv.conf", []byte(marmotResolvConfContent), 0644); err != nil {
		return fmt.Errorf("write /etc/resolv.conf: %w", err)
	}
	return nil
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
