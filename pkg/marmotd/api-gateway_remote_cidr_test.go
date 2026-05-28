package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestNormalizeGatewaySpec_DefaultRemoteCIDR(t *testing.T) {
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		ServerPorts:            []string{"80/tcp"},
	}

	if err := normalizeGatewaySpec(spec); err != nil {
		t.Fatalf("normalizeGatewaySpec() failed: %v", err)
	}

	if spec.RemoteCIDR != "0.0.0.0/0" {
		t.Fatalf("spec.RemoteCIDR = %q, want %q", spec.RemoteCIDR, "0.0.0.0/0")
	}
}

func TestNormalizeGatewaySpec_InvalidRemoteCIDR(t *testing.T) {
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDR:             "invalid-cidr",
		ServerPorts:            []string{"80/tcp"},
	}

	err := normalizeGatewaySpec(spec)
	if err == nil {
		t.Fatalf("normalizeGatewaySpec() expected error for invalid CIDR")
	}
}

func TestNormalizeGatewaySpec_ValidRemoteCIDR(t *testing.T) {
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDR:             " 10.0.0.0/24 ",
		ServerPorts:            []string{"80/tcp"},
	}

	if err := normalizeGatewaySpec(spec); err != nil {
		t.Fatalf("normalizeGatewaySpec() failed: %v", err)
	}

	if spec.RemoteCIDR != "10.0.0.0/24" {
		t.Fatalf("spec.RemoteCIDR = %q, want %q", spec.RemoteCIDR, "10.0.0.0/24")
	}
}

func TestNormalizeGatewaySpec_RejectsIPv6RemoteCIDR(t *testing.T) {
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDR:             "2001:db8::/64",
		ServerPorts:            []string{"80/tcp"},
	}

	err := normalizeGatewaySpec(spec)
	if err == nil {
		t.Fatalf("normalizeGatewaySpec() expected error for IPv6 CIDR")
	}
}
