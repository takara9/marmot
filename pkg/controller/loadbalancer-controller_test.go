package controller

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/util"
)

type mockLoadBalancerFabric struct {
	ensureCalls []networkfabric.OVNLoadBalancerSpec
	deleteCalls []struct {
		id  string
		lsw string
	}
}

func (m *mockLoadBalancerFabric) EnsureLoadBalancer(spec networkfabric.OVNLoadBalancerSpec) (string, error) {
	m.ensureCalls = append(m.ensureCalls, spec)
	return "marmot-lb-" + spec.LoadBalancerID, nil
}

func (m *mockLoadBalancerFabric) DeleteLoadBalancer(loadBalancerID string, logicalSwitchName string) error {
	m.deleteCalls = append(m.deleteCalls, struct {
		id  string
		lsw string
	}{id: loadBalancerID, lsw: logicalSwitchName})
	return nil
}

func (m *mockLoadBalancerFabric) GetLoadBalancerStatus(loadBalancerID string, logicalSwitchName string) (networkfabric.OVNLoadBalancerStatus, error) {
	return networkfabric.OVNLoadBalancerStatus{}, nil
}

func TestLoadBalancerControllerPendingToActive(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}, deletionDelay: 15}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-1", "web-servers", "172.16.10.2", true)

	lb, err := database.CreateLoadBalancer(api.LoadBalancer{
		ApiVersion: "v1",
		Kind:       "LoadBalancer",
		Metadata: api.Metadata{
			Name:     "lb-1",
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.LoadBalancerSpec{
			BackendMode:            util.StringPtr("auto"),
			InternalVirtualNetwork: "web-servers",
			ServerPorts:            []string{"80/tcp", "443/tcp"},
			VirtualIpAddress:       util.StringPtr("172.16.10.10"),
		},
	})
	if err != nil {
		t.Fatalf("CreateLoadBalancer() failed: %v", err)
	}
	lbID := api.LoadBalancerID(lb)

	mockFabric := &mockLoadBalancerFabric{}
	origFactory := newLoadBalancerFabric
	newLoadBalancerFabric = func() networkfabric.LoadBalancerFabric { return mockFabric }
	t.Cleanup(func() {
		newLoadBalancerFabric = origFactory
	})

	ctrl.loadBalancerControllerLoop()
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending loop: %v", err)
	}
	if afterPending.Status == nil || afterPending.Status.StatusCode != db.LOAD_BALANCER_PROVISIONING {
		t.Fatalf("status after first loop = %v, want %d(PROVISIONING)", afterPending.Status, db.LOAD_BALANCER_PROVISIONING)
	}

	ctrl.loadBalancerControllerLoop()
	afterProvisioning, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after provisioning loop: %v", err)
	}
	if afterProvisioning.Status == nil || afterProvisioning.Status.StatusCode != db.LOAD_BALANCER_ACTIVE {
		t.Fatalf("status after second loop = %v, want %d(ACTIVE)", afterProvisioning.Status, db.LOAD_BALANCER_ACTIVE)
	}
	if len(mockFabric.ensureCalls) == 0 {
		t.Fatalf("EnsureLoadBalancer() was not called")
	}
	lastCall := mockFabric.ensureCalls[len(mockFabric.ensureCalls)-1]
	if lastCall.LoadBalancerID != lbID {
		t.Fatalf("EnsureLoadBalancer().LoadBalancerID = %q, want %q", lastCall.LoadBalancerID, lbID)
	}
	if lastCall.LogicalSwitchName == "" {
		t.Fatalf("EnsureLoadBalancer().LogicalSwitchName is empty")
	}
	vipMap := lastCall.VIPs
	if len(vipMap) != 2 {
		t.Fatalf("EnsureLoadBalancer().VIPs len = %d, want 2; got=%v", len(vipMap), vipMap)
	}
	if got := vipMap["172.16.10.10:80"]; strings.TrimSpace(got) != "172.16.10.2:80" {
		t.Fatalf("vip mapping for 80 = %q, want %q", got, "172.16.10.2:80")
	}
	if got := vipMap["172.16.10.10:443"]; strings.TrimSpace(got) != "172.16.10.2:443" {
		t.Fatalf("vip mapping for 443 = %q, want %q", got, "172.16.10.2:443")
	}
}

func mustCreateLoadBalancerBackendServer(t *testing.T, database *db.Database, name, networkName, ipAddress string, enabled bool) api.Server {
	t.Helper()
	labels := map[string]interface{}{}
	if enabled {
		labels["lb-enabled"] = "true"
	}
	server, err := database.MakeServerEntry(api.Server{
		ApiVersion: "v1",
		Kind:       "Server",
		Metadata: api.Metadata{
			Name:     name,
			NodeName: util.StringPtr("hvc"),
			Labels:   &labels,
		},
		Spec: api.ServerSpec{
			NetworkInterface: &[]api.NetworkInterface{{
				Networkname: networkName,
				Address:     util.StringPtr(ipAddress),
			}},
		},
	})
	if err != nil {
		t.Fatalf("MakeServerEntry() failed for backend server: %v", err)
	}
	database.UpdateServerStatus(api.ServerID(server), db.SERVER_RUNNING, "")
	return server
}
