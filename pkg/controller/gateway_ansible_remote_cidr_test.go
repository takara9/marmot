package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
)

func TestGatewayRemoteCIDR_Default(t *testing.T) {
	if got := gatewayRemoteCIDR(api.GatewaySpec{}); got != "0.0.0.0/0" {
		t.Fatalf("gatewayRemoteCIDR() = %q, want %q", got, "0.0.0.0/0")
	}
}

func TestDesiredGatewayConfigHash_ChangesByRemoteCIDR(t *testing.T) {
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
	withCIDR1.Spec.RemoteCIDR = "10.0.0.0/24"
	withCIDR2 := base
	withCIDR2.Spec.RemoteCIDR = "192.168.0.0/16"

	hash1 := desiredGatewayConfigHash(withCIDR1, "172.16.10.2")
	hash2 := desiredGatewayConfigHash(withCIDR2, "172.16.10.2")

	if hash1 == hash2 {
		t.Fatalf("config hash should change when remoteCIDR changes: %q", hash1)
	}
}
