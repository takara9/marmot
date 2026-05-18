package internaldns

import (
	"errors"
	"net"
	"testing"
)

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
