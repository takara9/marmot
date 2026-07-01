package db

import (
	"net/netip"
	"testing"
)

func TestParseAndValidateAllocRangeAddr(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	networkAddr := prefix.Masked().Addr()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid host", input: "192.168.1.129", wantErr: false},
		{name: "network address", input: "192.168.1.0", wantErr: true},
		{name: "gateway address", input: "192.168.1.1", wantErr: true},
		{name: "broadcast address", input: "192.168.1.255", wantErr: true},
		{name: "outside prefix", input: "192.168.2.10", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAndValidateAllocRangeAddr(tc.input, prefix, networkAddr)
			if tc.wantErr && err == nil {
				t.Fatalf("parseAndValidateAllocRangeAddr(%q) expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("parseAndValidateAllocRangeAddr(%q) unexpected error: %v", tc.input, err)
			}
		})
	}
}
