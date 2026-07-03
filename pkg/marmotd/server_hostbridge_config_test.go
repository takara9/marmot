package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestHostBridgeConfigDefaults(t *testing.T) {
	orig := CurrentConfig()
	cfg := *orig
	cfg.HostBridgeDefault = &HostBridgeDefaultConfig{
		Netmasklen: 24,
		Nameservers: HostBridgeNameserversConfig{
			Addresses: []string{"8.8.8.8"},
			Search:    []string{"labo.local"},
		},
		Routes: []HostBridgeRouteConfig{{To: "default", Via: "192.168.1.1"}},
	}
	SetRuntimeConfig(&cfg)
	t.Cleanup(func() {
		SetRuntimeConfig(orig)
	})

	routes := hostBridgeRoutesFromConfig()
	if routes == nil || len(*routes) != 1 {
		t.Fatalf("hostBridgeRoutesFromConfig() = %v, want 1 route", routes)
	}
	if (*routes)[0].To == nil || *(*routes)[0].To != "default" {
		t.Fatalf("route to = %v, want default", (*routes)[0].To)
	}

	ns := hostBridgeNameserversFromConfig()
	if ns == nil || ns.Addresses == nil || len(*ns.Addresses) != 1 {
		t.Fatalf("hostBridgeNameserversFromConfig() = %v, want one address", ns)
	}
	if ns.Search == nil || len(*ns.Search) != 1 || (*ns.Search)[0] != "labo.local" {
		t.Fatalf("search domains = %v, want [labo.local]", ns.Search)
	}

	nic := api.NetworkInterface{}
	applyHostBridgeDefaultsFromConfig(&nic)
	if nic.Netmasklen == nil || *nic.Netmasklen != 24 {
		t.Fatalf("netmasklen = %v, want 24", nic.Netmasklen)
	}
	if nic.Routes == nil || len(*nic.Routes) != 1 {
		t.Fatalf("routes = %v, want one route", nic.Routes)
	}
	if nic.Nameservers == nil || nic.Nameservers.Addresses == nil || len(*nic.Nameservers.Addresses) != 1 {
		t.Fatalf("nameservers = %v, want one address", nic.Nameservers)
	}
}

func TestEnsureHostBridgeIPNetworkRequiresConfig(t *testing.T) {
	orig := CurrentConfig()
	cfg := *orig
	cfg.HostBridgeIPNetAddr = ""
	cfg.HostBridgeIPAddrStart = ""
	cfg.HostBridgeIPAddrEnd = ""
	SetRuntimeConfig(&cfg)
	t.Cleanup(func() {
		SetRuntimeConfig(orig)
	})

	m := &Marmot{}
	vnet := &api.VirtualNetwork{}
	api.SetVirtualNetworkID(vnet, "abcde")
	vnet.Spec.IpNetworkId = util.StringPtr("")

	_, err := m.ensureHostBridgeIPNetwork(vnet)
	if err == nil {
		t.Fatal("ensureHostBridgeIPNetwork() error = nil, want error")
	}
}
