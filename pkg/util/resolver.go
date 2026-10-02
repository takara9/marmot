package util

import (
	"net"
	"strings"
)

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
