package controller

import (
	"reflect"
	"sort"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestCollectNetworkDeleteDependencies_MatchesByNameAndID(t *testing.T) {
	networkName := "app-net"
	networkID := "n1234"

	servers := []api.Server{
		{
			Metadata: api.Metadata{Name: "web-1"},
			Spec:     api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{Networkname: "other-net", Networkid: "x1"}}},
		},
		{
			Metadata: api.Metadata{Name: "web-2"},
			Spec:     api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{Networkname: "app-net", Networkid: "x2"}}},
		},
		{
			Metadata: api.Metadata{Name: "web-3"},
			Spec:     api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{Networkname: "mismatch", Networkid: "n1234"}}},
		},
	}

	loadBalancers := []api.ApplicationLoadBalancer{
		{Metadata: api.Metadata{Name: "alb-app"}, Spec: api.ApplicationLoadBalancerSpec{InternalVirtualNetwork: "app-net"}},
		{Metadata: api.Metadata{Name: "alb-other"}, Spec: api.ApplicationLoadBalancerSpec{InternalVirtualNetwork: "other-net"}},
	}
	gateways := []api.Gateway{
		{Metadata: api.Metadata{Name: "gw-app"}, Spec: api.GatewaySpec{InternalVirtualNetwork: "app-net"}},
	}
	vpnGateways := []api.VpnGateway{
		{Metadata: api.Metadata{Name: "vpngw-app"}, Spec: api.VpnGatewaySpec{InternalVirtualNetwork: "app-net"}},
	}

	deps := collectNetworkDeleteDependencies(networkName, networkID, servers, loadBalancers, gateways, vpnGateways)
	if !deps.hasAny() {
		t.Fatalf("expected dependencies to be detected")
	}

	sort.Strings(deps.servers)
	if !reflect.DeepEqual(deps.servers, []string{"web-2", "web-3"}) {
		t.Fatalf("unexpected servers: got=%v", deps.servers)
	}
	if !reflect.DeepEqual(deps.loadBalancers, []string{"alb-app"}) {
		t.Fatalf("unexpected load balancers: got=%v", deps.loadBalancers)
	}
	if !reflect.DeepEqual(deps.gateways, []string{"gw-app"}) {
		t.Fatalf("unexpected gateways: got=%v", deps.gateways)
	}
	if !reflect.DeepEqual(deps.vpnGateways, []string{"vpngw-app"}) {
		t.Fatalf("unexpected vpn gateways: got=%v", deps.vpnGateways)
	}
}

func TestCollectNetworkDeleteDependencies_NoDependents(t *testing.T) {
	deps := collectNetworkDeleteDependencies("app-net", "n1234", nil, nil, nil, nil)
	if deps.hasAny() {
		t.Fatalf("expected no dependencies, got=%+v", deps)
	}
	if got, want := deps.statusMessage(), "deletion:blocked-by-dependents servers=0 loadBalancers=0 gateways=0 vpnGateways=0"; got != want {
		t.Fatalf("unexpected status message: got=%q want=%q", got, want)
	}
}
