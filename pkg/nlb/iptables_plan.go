package nlb

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

type Backend struct {
	Address string
	Port    int
}

type Listener struct {
	Name               string
	Protocol           string
	VipPort            int
	SessionPersistence bool
	Backends           []Backend
}

type IPTablesPlanInput struct {
	BindPublicIP string
	ChainPrefix  string
	RemoteCIDR   string
	Listeners    []Listener
}

// BuildIPTablesRestoreScript builds iptables-restore input for simple L4 DNAT load balancing.
// The plan relies on conntrack NAT state so once a flow is translated it stays pinned to the
// selected backend for the flow lifetime.
func BuildIPTablesRestoreScript(input IPTablesPlanInput) (string, error) {
	publicIP, err := normalizeIP(input.BindPublicIP)
	if err != nil {
		return "", fmt.Errorf("invalid bind public ip: %w", err)
	}

	remoteCIDR, err := normalizeCIDR(input.RemoteCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid remote cidr: %w", err)
	}

	if len(input.Listeners) == 0 {
		return "", fmt.Errorf("at least one listener is required")
	}

	chainPrefix := strings.TrimSpace(input.ChainPrefix)
	if chainPrefix == "" {
		chainPrefix = "NLB"
	}

	var b strings.Builder
	b.WriteString("*nat\n")
	b.WriteString(":PREROUTING ACCEPT [0:0]\n")
	b.WriteString(":INPUT ACCEPT [0:0]\n")
	b.WriteString(":OUTPUT ACCEPT [0:0]\n")
	b.WriteString(":POSTROUTING ACCEPT [0:0]\n")

	listenerNames := make(map[string]struct{}, len(input.Listeners))
	for index, listener := range input.Listeners {
		if err := validateListener(listener); err != nil {
			return "", fmt.Errorf("listener[%d] %q: %w", index, listener.Name, err)
		}
		if _, exists := listenerNames[listener.Name]; exists {
			return "", fmt.Errorf("listener[%d] %q: duplicate listener name", index, listener.Name)
		}
		listenerNames[listener.Name] = struct{}{}

		chainName := fmt.Sprintf("%s_%d", chainPrefix, index)
		b.WriteString(fmt.Sprintf(":%s - [0:0]\n", chainName))
		b.WriteString(
			fmt.Sprintf(
				"-A PREROUTING -d %s -s %s -p %s --dport %d -m conntrack --ctstate NEW -j %s\n",
				publicIP,
				remoteCIDR,
				strings.ToLower(listener.Protocol),
				listener.VipPort,
				chainName,
			),
		)

		if listener.SessionPersistence {
			b.WriteString(fmt.Sprintf("# %s: sessionPersistence=true uses conntrack flow pinning\n", chainName))
		}

		for backendIndex, backend := range listener.Backends {
			if backendIndex == len(listener.Backends)-1 {
				b.WriteString(
					fmt.Sprintf(
						"-A %s -j DNAT --to-destination %s:%d\n",
						chainName,
						backend.Address,
						backend.Port,
					),
				)
				continue
			}

			probability := 1.0 / float64(len(listener.Backends)-backendIndex)
			b.WriteString(
				fmt.Sprintf(
					"-A %s -m statistic --mode random --probability %.10f -j DNAT --to-destination %s:%d\n",
					chainName,
					probability,
					backend.Address,
					backend.Port,
				),
			)
		}
	}

	b.WriteString("COMMIT\n")
	return b.String(), nil
}

func validateListener(listener Listener) error {
	if strings.TrimSpace(listener.Name) == "" {
		return fmt.Errorf("name is required")
	}
	protocol := strings.ToLower(strings.TrimSpace(listener.Protocol))
	if protocol != "tcp" && protocol != "udp" {
		return fmt.Errorf("protocol must be tcp or udp")
	}
	if listener.VipPort < 1 || listener.VipPort > 65535 {
		return fmt.Errorf("vipPort must be 1-65535")
	}
	if len(listener.Backends) == 0 {
		return fmt.Errorf("at least one backend is required")
	}

	seen := map[string]struct{}{}
	for i, backend := range listener.Backends {
		ip, err := normalizeIP(backend.Address)
		if err != nil {
			return fmt.Errorf("backend[%d] invalid address: %w", i, err)
		}
		if backend.Port < 1 || backend.Port > 65535 {
			return fmt.Errorf("backend[%d] port must be 1-65535", i)
		}
		key := fmt.Sprintf("%s:%d", ip, backend.Port)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("backend[%d] duplicate endpoint", i)
		}
		seen[key] = struct{}{}
		listener.Backends[i].Address = ip
	}

	return nil
}

func normalizeIP(raw string) (string, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if !ip.Is4() {
		return "", fmt.Errorf("only IPv4 is supported in initial implementation")
	}
	return ip.String(), nil
}

func normalizeCIDR(raw string) (string, error) {
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

func SortedBackendEndpoints(backends []Backend) []string {
	out := make([]string, 0, len(backends))
	for _, backend := range backends {
		out = append(out, fmt.Sprintf("%s:%d", strings.TrimSpace(backend.Address), backend.Port))
	}
	sort.Strings(out)
	return out
}
