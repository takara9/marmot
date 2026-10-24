package marmotd

import "testing"

func TestResolveSpiceListenAddress(t *testing.T) {
	tests := []struct {
		name    string
		adv     string
		dns     string
		want    string
		wantErr bool
	}{
		{"advertise preferred", "192.168.1.10", "0.0.0.0:53", "192.168.1.10", false},
		{"wildcard dns without advertise", "", "0.0.0.0:53", "", true},
		{"ipv6 wildcard dns without advertise", "", "[::]:53", "", true},
		{"concrete dns listener", "", "10.0.0.5:53", "10.0.0.5", false},
		{"invalid advertise", "not-an-ip", "10.0.0.5:53", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSpiceListenAddress(&MarmotdConfig{APIAdvertiseHostBridgeAddress: tt.adv, DNSListenAddr: tt.dns})
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %q err=%v, want %q wantErr=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
