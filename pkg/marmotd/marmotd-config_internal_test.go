package marmotd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/takara9/marmot/pkg/db"
)

func TestResolveDNSListenAddrFromInterfaces_UsesFirstIPv4AfterLo(t *testing.T) {
	origListInterfaces := listInterfaces
	origListInterfaceAddrs := listInterfaceAddrs
	t.Cleanup(func() {
		listInterfaces = origListInterfaces
		listInterfaceAddrs = origListInterfaceAddrs
	})

	listInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			{Name: "lo", Index: 1},
			{Name: "eth0", Index: 2},
			{Name: "eth1", Index: 3},
		}, nil
	}
	listInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		switch iface.Name {
		case "eth0":
			return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.122.10")}}, nil
		case "eth1":
			return []net.Addr{&net.IPNet{IP: net.ParseIP("10.0.0.10")}}, nil
		default:
			return nil, nil
		}
	}

	addr, ok := resolveDNSListenAddrFromInterfaces()
	if !ok {
		t.Fatalf("expected resolved address")
	}
	if addr != "192.168.122.10:53" {
		t.Fatalf("unexpected address: %s", addr)
	}
}

func TestNormalizeConfig_DNSListenAddrEmpty_FallsBackToDefaultWhenUnresolved(t *testing.T) {
	origListInterfaces := listInterfaces
	origListInterfaceAddrs := listInterfaceAddrs
	t.Cleanup(func() {
		listInterfaces = origListInterfaces
		listInterfaceAddrs = origListInterfaceAddrs
	})

	listInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "lo", Index: 1}}, nil
	}
	listInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return nil, nil
	}

	cfg := normalizeConfig(&MarmotdConfig{DNSListenAddr: ""})
	if cfg.DNSListenAddr != "0.0.0.0:53" {
		t.Fatalf("expected default dns listen addr, got: %s", cfg.DNSListenAddr)
	}
}

func TestNormalizeConfig_DNSListenAddrEmpty_UsesInterfaceIPv4(t *testing.T) {
	origListInterfaces := listInterfaces
	origListInterfaceAddrs := listInterfaceAddrs
	t.Cleanup(func() {
		listInterfaces = origListInterfaces
		listInterfaceAddrs = origListInterfaceAddrs
	})

	listInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "lo", Index: 1}, {Name: "ens3", Index: 2}}, nil
	}
	listInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		if iface.Name == "ens3" {
			return []net.Addr{&net.IPNet{IP: net.ParseIP("10.10.0.5")}}, nil
		}
		return nil, nil
	}

	cfg := normalizeConfig(&MarmotdConfig{DNSListenAddr: ""})
	if cfg.DNSListenAddr != "10.10.0.5:53" {
		t.Fatalf("expected interface-based dns listen addr, got: %s", cfg.DNSListenAddr)
	}
}

func TestParseSessionIdleTimeout(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "minutes", input: "30m", want: 30 * time.Minute},
		{name: "hours", input: "24h", want: 24 * time.Hour},
		{name: "days", input: "3d", want: 72 * time.Hour},
		{name: "trim and uppercase", input: " 2H ", want: 2 * time.Hour},
		{name: "invalid unit", input: "10w", wantErr: true},
		{name: "missing unit", input: "10", wantErr: true},
		{name: "zero", input: "0h", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSessionIdleTimeout(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("unexpected duration for input %q: got %s want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetRuntimeConfig_AppliesSessionIdleTimeout(t *testing.T) {
	origCfg := CurrentConfig()
	origTimeout := db.GetAuthSessionIdleTimeout()
	t.Cleanup(func() {
		SetRuntimeConfig(origCfg)
		_ = db.SetAuthSessionIdleTimeout(origTimeout)
	})

	cfg := *origCfg
	cfg.SessionIdleTimeout = "2h"
	SetRuntimeConfig(&cfg)

	if got := db.GetAuthSessionIdleTimeout(); got != 2*time.Hour {
		t.Fatalf("unexpected auth idle timeout: got %s want %s", got, 2*time.Hour)
	}
}

func TestLoadConfig_SessionIdleTimeout(t *testing.T) {
	t.Run("defaults to 1h when omitted", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "marmotd.json")
		if err := os.WriteFile(path, []byte(`{"node_name":"hv1"}`), 0o644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error loading config: %v", err)
		}
		if cfg.SessionIdleTimeout != "1h" {
			t.Fatalf("unexpected default session_idle_timeout: got %q want %q", cfg.SessionIdleTimeout, "1h")
		}
	})

	t.Run("accepts d unit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "marmotd.json")
		if err := os.WriteFile(path, []byte(`{"session_idle_timeout":"3d"}`), 0o644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("unexpected error loading config: %v", err)
		}
		if cfg.SessionIdleTimeout != "3d" {
			t.Fatalf("unexpected session_idle_timeout: got %q want %q", cfg.SessionIdleTimeout, "3d")
		}
	})

	t.Run("rejects invalid unit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "marmotd.json")
		if err := os.WriteFile(path, []byte(`{"session_idle_timeout":"30x"}`), 0o644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("expected error for invalid session_idle_timeout")
		}
	})
}
