package controller

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestBuildVpnGatewayServerSpecSetsDefaultPublicRoute(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{
		db:     database,
		marmot: &marmotd.Marmot{NodeName: "hvc", Db: database},
	}

	hostBridge, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "host-bridge"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("10.10.0.0/24"),
		},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork(host-bridge) failed: %v", err)
	}
	ipNetID, err := database.CreateIpNetwork(api.VirtualNetworkID(hostBridge), &api.IPNetwork{AddressMaskLen: util.StringPtr("10.10.0.0/24")})
	if err != nil {
		t.Fatalf("CreateIpNetwork(host-bridge) failed: %v", err)
	}
	hostBridge.Spec.IpNetworkId = util.StringPtr(ipNetID)
	if err := database.UpdateVirtualNetworkById(api.VirtualNetworkID(hostBridge), hostBridge); err != nil {
		t.Fatalf("UpdateVirtualNetworkById(host-bridge) failed: %v", err)
	}

	if _, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "app-net"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("172.16.20.0/24"),
		},
	}); err != nil {
		t.Fatalf("CreateVirtualNetwork(app-net) failed: %v", err)
	}

	vpnGateway := api.VpnGateway{
		ApiVersion: "v1",
		Kind:       "VpnGateway",
		Metadata:   api.Metadata{Name: "vpn-gw"},
		Spec: api.VpnGatewaySpec{
			BindPublicIpAddress:    "10.10.0.40",
			InternalVirtualNetwork: "app-net",
		},
	}

	serverSpec, err := ctrl.buildVpnGatewayServerSpec(vpnGateway, "vgw-vpn-gw")
	if err != nil {
		t.Fatalf("buildVpnGatewayServerSpec() failed: %v", err)
	}
	if serverSpec.Spec.NetworkInterface == nil || len(*serverSpec.Spec.NetworkInterface) == 0 {
		t.Fatalf("NetworkInterface is empty")
	}

	publicNIC := (*serverSpec.Spec.NetworkInterface)[0]
	if publicNIC.Routes == nil || len(*publicNIC.Routes) == 0 {
		t.Fatalf("public NIC routes are empty")
	}
	route := (*publicNIC.Routes)[0]
	if route.To == nil || strings.TrimSpace(*route.To) != "default" {
		t.Fatalf("public NIC default route To = %v, want default", route.To)
	}
	if route.Via == nil || strings.TrimSpace(*route.Via) != "10.10.0.1" {
		t.Fatalf("public NIC default route Via = %v, want 10.10.0.1", route.Via)
	}
}

func TestBuildVpnGatewayServerSpecUsesCustomRoutes(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{
		db:     database,
		marmot: &marmotd.Marmot{NodeName: "hvc", Db: database},
	}

	hostBridge, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "host-bridge"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("10.10.0.0/24"),
		},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork(host-bridge) failed: %v", err)
	}
	ipNetID, err := database.CreateIpNetwork(api.VirtualNetworkID(hostBridge), &api.IPNetwork{AddressMaskLen: util.StringPtr("10.10.0.0/24")})
	if err != nil {
		t.Fatalf("CreateIpNetwork(host-bridge) failed: %v", err)
	}
	hostBridge.Spec.IpNetworkId = util.StringPtr(ipNetID)
	if err := database.UpdateVirtualNetworkById(api.VirtualNetworkID(hostBridge), hostBridge); err != nil {
		t.Fatalf("UpdateVirtualNetworkById(host-bridge) failed: %v", err)
	}

	if _, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "app-net"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("172.16.20.0/24"),
		},
	}); err != nil {
		t.Fatalf("CreateVirtualNetwork(app-net) failed: %v", err)
	}

	to := "default"
	via := "10.10.0.254"
	vpnGateway := api.VpnGateway{
		ApiVersion: "v1",
		Kind:       "VpnGateway",
		Metadata:   api.Metadata{Name: "vpn-gw"},
		Spec: api.VpnGatewaySpec{
			BindPublicIpAddress:    "10.10.0.40",
			InternalVirtualNetwork: "app-net",
			Routes:                 &[]api.Route{{To: &to, Via: &via}},
		},
	}

	serverSpec, err := ctrl.buildVpnGatewayServerSpec(vpnGateway, "vgw-vpn-gw")
	if err != nil {
		t.Fatalf("buildVpnGatewayServerSpec() failed: %v", err)
	}
	publicNIC := (*serverSpec.Spec.NetworkInterface)[0]
	if publicNIC.Routes == nil || len(*publicNIC.Routes) != 1 {
		t.Fatalf("public NIC routes = %v, want 1 route", publicNIC.Routes)
	}
	if gotVia := strings.TrimSpace(util.OrDefault((*publicNIC.Routes)[0].Via, "")); gotVia != via {
		t.Fatalf("public NIC route via = %q, want %q", gotVia, via)
	}
}