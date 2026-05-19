package marmotd

import (
	"net"
	"testing"
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
