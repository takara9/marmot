package internaldns

import (
	"errors"
	"net"
	"net/netip"
	"testing"
)

type stubAddr struct {
	network string
	addr    string
}

func (a stubAddr) Network() string { return a.network }

func (a stubAddr) String() string { return a.addr }

func TestDecodeDNSRecordIP(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		wantIP  net.IP
		wantStr string
		wantErr error
	}{
		{
			name:    "valid IPv4 JSON",
			raw:     []byte(`"192.168.100.2"`),
			wantIP:  net.ParseIP("192.168.100.2"),
			wantStr: "192.168.100.2",
		},
		{
			name:    "valid IPv6 JSON",
			raw:     []byte(`"2001:db8::10"`),
			wantIP:  net.ParseIP("2001:db8::10"),
			wantStr: "2001:db8::10",
		},
		{
			name:    "invalid JSON",
			raw:     []byte(`192.168.100.2`),
			wantErr: errors.New("json decode error"),
		},
		{
			name:    "invalid IP string",
			raw:     []byte(`"not-an-ip"`),
			wantStr: "not-an-ip",
			wantErr: errInvalidDNSRecordIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, gotStr, err := decodeDNSRecordIP(tt.raw)

			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if errors.Is(tt.wantErr, errInvalidDNSRecordIP) {
					if !errors.Is(err, errInvalidDNSRecordIP) {
						t.Fatalf("expected errInvalidDNSRecordIP, got: %v", err)
					}
				} else {
					if errors.Is(err, errInvalidDNSRecordIP) {
						t.Fatalf("unexpected errInvalidDNSRecordIP")
					}
				}
			}

			if tt.wantStr != gotStr {
				t.Fatalf("decoded string mismatch: want=%q got=%q", tt.wantStr, gotStr)
			}

			if tt.wantIP == nil && gotIP != nil {
				t.Fatalf("expected nil IP, got %v", gotIP)
			}
			if tt.wantIP != nil && !tt.wantIP.Equal(gotIP) {
				t.Fatalf("IP mismatch: want=%v got=%v", tt.wantIP, gotIP)
			}
		})
	}
}

func TestDomainToMarmotPath(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{domain: "vm1.test-net-1.", want: "/marmot/dns/test-net-1/vm1"},
		{domain: "api.example.local", want: "/marmot/dns/local/example/api"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := DomainToMarmotPath(tt.domain)
			if got != tt.want {
				t.Fatalf("path mismatch: want=%q got=%q", tt.want, got)
			}
		})
	}
}

func TestShouldForwardUpstream(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want bool
	}{
		{
			name: "loopback IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53000},
			want: true,
		},
		{
			name: "loopback IPv6",
			addr: &net.UDPAddr{IP: net.ParseIP("::1"), Port: 53000},
			want: true,
		},
		{
			name: "non loopback IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.10"), Port: 53000},
			want: false,
		},
		{
			name: "malformed addr string",
			addr: stubAddr{network: "udp", addr: "bad-addr"},
			want: false,
		},
		{
			name: "nil addr",
			addr: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldForwardUpstream(tt.addr, nil)
			if got != tt.want {
				t.Fatalf("forward mismatch: want=%v got=%v", tt.want, got)
			}
		})
	}
}


func TestShouldForwardUpstreamWithAllowlist(t *testing.T) {
	allowed := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
		netip.MustParsePrefix("fd00::/64"),
	}

	tests := []struct {
		name string
		addr net.Addr
		want bool
	}{
		{
			name: "allowed IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("192.168.1.10"), Port: 53000},
			want: true,
		},
		{
			name: "allowed IPv6 ula",
			addr: &net.UDPAddr{IP: net.ParseIP("fd00::10"), Port: 53000},
			want: true,
		},
		{
			name: "not listed private IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("10.0.0.10"), Port: 53000},
			want: false,
		},
		{
			name: "not listed link local IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("169.254.10.20"), Port: 53000},
			want: false,
		},
		{
			name: "public IPv4",
			addr: &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53000},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldForwardUpstream(tt.addr, allowed)
			if got != tt.want {
				t.Fatalf("forward mismatch: want=%v got=%v", tt.want, got)
			}
		})
	}
}

func TestParseAllowedUpstreamCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		want    []netip.Prefix
		wantErr bool
	}{
		{
			name:  "valid list",
			cidrs: []string{"192.168.1.12/24", "fd00::1/64"},
			want: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
				netip.MustParsePrefix("fd00::/64"),
			},
		},
		{
			name:    "invalid cidr",
			cidrs:   []string{"not-a-cidr"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAllowedUpstreamCIDRs(tt.cidrs)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("prefix count mismatch: want=%d got=%d", len(tt.want), len(got))
			}
			for index := range tt.want {
				if got[index] != tt.want[index] {
					t.Fatalf("prefix mismatch at %d: want=%s got=%s", index, tt.want[index], got[index])
				}
			}
		})
	}
}
