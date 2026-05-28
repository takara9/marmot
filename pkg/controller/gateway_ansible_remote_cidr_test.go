package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestGatewayRemoteCIDRs_Default(t *testing.T) {
	got := gatewayRemoteCIDRs(api.GatewaySpec{})
	if len(got) != 1 || got[0] != "0.0.0.0/0" {
		t.Fatalf("gatewayRemoteCIDRs() = %v, want [0.0.0.0/0]", got)
	}
}

func TestDesiredGatewayConfigHash_ChangesByRemoteCIDRs(t *testing.T) {
	base := api.Gateway{
		Metadata: api.Metadata{Id: "gw-1"},
		Spec: api.GatewaySpec{
			BindPublicIpAddress:    "192.168.1.100",
			InternalServerName:     "web-1",
			InternalVirtualNetwork: "net-web",
			ServerPorts:            []string{"80/tcp", "443/tcp"},
		},
	}
	withCIDR1 := base
	withCIDR1.Spec.RemoteCIDRs = []string{"10.0.0.0/24"}
	withCIDR2 := base
	withCIDR2.Spec.RemoteCIDRs = []string{"192.168.0.0/16"}

	hash1 := desiredGatewayConfigHash(withCIDR1, "172.16.10.2")
	hash2 := desiredGatewayConfigHash(withCIDR2, "172.16.10.2")

	if hash1 == hash2 {
		t.Fatalf("config hash should change when remoteCIDR changes: %q", hash1)
	}
}
