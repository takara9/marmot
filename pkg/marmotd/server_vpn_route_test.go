package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestFirstHostAddressFromCIDR(t *testing.T) {
	got, err := firstHostAddressFromCIDR("172.16.0.0/24")
	if err != nil {
		t.Fatalf("firstHostAddressFromCIDR() failed: %v", err)
	}
	if got != "172.16.0.1" {
		t.Fatalf("firstHostAddressFromCIDR() = %q, want %q", got, "172.16.0.1")
	}
}

func TestAppendVPNRouteIfEnabled(t *testing.T) {
	nic := &api.NetworkInterface{}
	vpnOn := true
	vnet := &api.VirtualNetwork{Spec: api.VirtualNetworkSpec{IPNetworkAddress: util.StringPtr("172.16.0.0/24"), VpnAccess: &vpnOn}}

	appendVPNRouteIfEnabled(nic, vnet)
	if nic.Routes == nil || len(*nic.Routes) != 1 {
		t.Fatalf("routes = %v, want one route", nic.Routes)
	}
	r := (*nic.Routes)[0]
	if r.To == nil || *r.To != "10.8.0.0/24" {
		t.Fatalf("route.to = %v, want 10.8.0.0/24", r.To)
	}
	if r.Via == nil || *r.Via != "172.16.0.1" {
		t.Fatalf("route.via = %v, want 172.16.0.1", r.Via)
	}

	appendVPNRouteIfEnabled(nic, vnet)
	if len(*nic.Routes) != 1 {
		t.Fatalf("duplicate route was appended: %v", *nic.Routes)
	}
}
