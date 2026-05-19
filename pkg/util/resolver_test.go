package util

import "testing"

func TestNameserverForDNSListenAddr(t *testing.T) {
	tests := []struct {
		name       string
		listenAddr string
		want       string
	}{
		{name: "wildcard ipv4", listenAddr: "0.0.0.0:53", want: "127.0.0.1"},
		{name: "loopback ipv4", listenAddr: "127.0.0.1:53", want: "127.0.0.1"},
		{name: "specific ipv4", listenAddr: "192.168.122.10:53", want: "192.168.122.10"},
		{name: "wildcard ipv6", listenAddr: "[::]:53", want: "::1"},
		{name: "specific ipv6", listenAddr: "[2001:db8::1]:53", want: "2001:db8::1"},
		{name: "invalid format fallback", listenAddr: "invalid", want: "127.0.0.1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nameserverForDNSListenAddr(tc.listenAddr)
			if got != tc.want {
				t.Fatalf("nameserverForDNSListenAddr(%q) = %q, want %q", tc.listenAddr, got, tc.want)
			}
		})
	}
}
