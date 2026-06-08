package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestNormalizeGatewaySpec_DefaultRemoteCIDRs(t *testing.T) {
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		ServerPorts:            []string{"80/tcp"},
	}

	if err := normalizeGatewaySpec(spec); err != nil {
		t.Fatalf("normalizeGatewaySpec() failed: %v", err)
	}

	if spec.RemoteCIDRs != nil && len(*spec.RemoteCIDRs) != 0 {
		t.Fatalf("spec.RemoteCIDRs = %v, want empty", spec.RemoteCIDRs)
	}
}

func TestNormalizeGatewaySpec_InvalidRemoteCIDRs(t *testing.T) {
	cidrs := []string{"invalid-cidr"}
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDRs:            &cidrs,
		ServerPorts:            []string{"80/tcp"},
	}

	err := normalizeGatewaySpec(spec)
	if err == nil {
		t.Fatalf("normalizeGatewaySpec() expected error for invalid CIDR")
	}
}

func TestNormalizeGatewaySpec_ValidRemoteCIDRs(t *testing.T) {
	cidrs := []string{" 10.0.0.0/24 ", "10.0.0.0/24", "192.168.0.0/16"}
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDRs:            &cidrs,
		ServerPorts:            []string{"80/tcp"},
	}

	if err := normalizeGatewaySpec(spec); err != nil {
		t.Fatalf("normalizeGatewaySpec() failed: %v", err)
	}

	if len(*spec.RemoteCIDRs) != 2 {
		t.Fatalf("spec.RemoteCIDRs len = %d, want 2", len(*spec.RemoteCIDRs))
	}
	if (*spec.RemoteCIDRs)[0] != "10.0.0.0/24" || (*spec.RemoteCIDRs)[1] != "192.168.0.0/16" {
		t.Fatalf("spec.RemoteCIDRs = %v, want [10.0.0.0/24 192.168.0.0/16]", spec.RemoteCIDRs)
	}
	if *spec.RemoteCIDR != "10.0.0.0/24" {
		t.Fatalf("spec.RemoteCIDR = %q, want %q", *spec.RemoteCIDR, "10.0.0.0/24")
	}
}

func TestNormalizeGatewaySpec_RejectsIPv6RemoteCIDRs(t *testing.T) {
	cidrs := []string{"2001:db8::/64"}
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDRs:            &cidrs,
		ServerPorts:            []string{"80/tcp"},
	}

	err := normalizeGatewaySpec(spec)
	if err == nil {
		t.Fatalf("normalizeGatewaySpec() expected error for IPv6 CIDR")
	}
}

func TestNormalizeGatewaySpec_LegacyRemoteCIDRMappedToRemoteCIDRs(t *testing.T) {
	remoteCIDR := "10.10.0.0/16"
	spec := &api.GatewaySpec{
		BindPublicIpAddress:    "192.168.1.100",
		InternalServerName:     "web-1",
		InternalVirtualNetwork: "net-web",
		RemoteCIDR:             &remoteCIDR,
		ServerPorts:            []string{"80/tcp"},
	}

	if err := normalizeGatewaySpec(spec); err != nil {
		t.Fatalf("normalizeGatewaySpec() failed: %v", err)
	}

	if len(*spec.RemoteCIDRs) != 1 || (*spec.RemoteCIDRs)[0] != "10.10.0.0/16" {
		t.Fatalf("spec.RemoteCIDRs = %v, want [10.10.0.0/16]", spec.RemoteCIDRs)
	}
}
