package marmotd

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// NormalizeServerPorts normalizes spec.serverPorts entries into "<port>/<proto>" format.
// Service names are resolved with tcp as default protocol.
func NormalizeServerPorts(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("spec.serverPorts is required")
	}

	normalized := make([]string, 0, len(raw))
	seen := map[string]struct{}{}

	for _, p := range raw {
		entry := strings.TrimSpace(p)
		if entry == "" {
			return nil, fmt.Errorf("spec.serverPorts contains an empty entry")
		}

		portSpec, err := resolveServerPortSpec(entry)
		if err != nil {
			return nil, err
		}

		if _, ok := seen[portSpec]; ok {
			continue
		}
		seen[portSpec] = struct{}{}
		normalized = append(normalized, portSpec)
	}

	if len(normalized) == 0 {
		return nil, fmt.Errorf("spec.serverPorts is required")
	}

	return normalized, nil
}

func resolveServerPortSpec(entry string) (string, error) {
	if strings.Contains(entry, "/") {
		parts := strings.Split(entry, "/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid port format %q", entry)
		}
		port, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid port number in %q", entry)
		}
		proto := strings.ToLower(strings.TrimSpace(parts[1]))
		if proto != "tcp" && proto != "udp" {
			return "", fmt.Errorf("invalid protocol in %q: must be tcp or udp", entry)
		}
		return fmt.Sprintf("%d/%s", port, proto), nil
	}

	if _, err := strconv.Atoi(entry); err == nil {
		return "", fmt.Errorf("numeric port %q must include protocol suffix like /tcp or /udp", entry)
	}

	port, err := net.LookupPort("tcp", strings.ToLower(entry))
	if err != nil {
		return "", fmt.Errorf("failed to resolve service name %q with tcp: %w", entry, err)
	}
	return fmt.Sprintf("%d/tcp", port), nil
}
