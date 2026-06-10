package marmotd

import "testing"

func TestDefaultNameserversFromConfig_UsesPublicDNSForWildcardAndLoopbackListenAddr(t *testing.T) {
	orig := CurrentConfig()
	t.Cleanup(func() {
		SetRuntimeConfig(orig)
	})

	tests := []struct {
		name          string
		dnsListenAddr string
		want          []string
	}{
		{
			name:          "wildcard listen addr",
			dnsListenAddr: "0.0.0.0:53",
			want:          []string{"8.8.8.8"},
		},
		{
			name:          "loopback listen addr",
			dnsListenAddr: "127.0.0.1:53",
			want:          []string{"8.8.8.8"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *orig
			cfg.DNSListenAddr = tc.dnsListenAddr
			cfg.DNSUpstream = "8.8.8.8:53"
			SetRuntimeConfig(&cfg)

			ns := defaultNameserversFromConfig()
			if ns == nil || ns.Addresses == nil {
				t.Fatalf("defaultNameserversFromConfig() = nil, want %v", tc.want)
			}
			got := *ns.Addresses
			if len(got) != len(tc.want) {
				t.Fatalf("nameserver count = %d, want %d, got=%v", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("nameserver[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestDefaultNameserversFromConfig_UsesListenAddrWhenNotWildcardOrLoopback(t *testing.T) {
	orig := CurrentConfig()
	t.Cleanup(func() {
		SetRuntimeConfig(orig)
	})

	cfg := *orig
	cfg.DNSListenAddr = "192.168.122.10:53"
	cfg.DNSUpstream = "1.1.1.1:53"
	SetRuntimeConfig(&cfg)

	ns := defaultNameserversFromConfig()
	if ns == nil || ns.Addresses == nil {
		t.Fatalf("defaultNameserversFromConfig() = nil")
	}
	got := *ns.Addresses
	want := []string{"192.168.122.10", "1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("nameserver count = %d, want %d, got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("nameserver[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
