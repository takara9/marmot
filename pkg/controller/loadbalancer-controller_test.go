package controller

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/util"
)

type mockLoadBalancerFabric struct {
	ensureCalls []networkfabric.OVNLoadBalancerSpec
	ensureErr   error
	deleteCalls []struct {
		id  string
		lsw string
	}
	deleteErr error
}

func (m *mockLoadBalancerFabric) EnsureLoadBalancer(spec networkfabric.OVNLoadBalancerSpec) (string, error) {
	m.ensureCalls = append(m.ensureCalls, spec)
	if m.ensureErr != nil {
		return "", m.ensureErr
	}
	return "marmot-lb-" + spec.LoadBalancerID, nil
}

func (m *mockLoadBalancerFabric) DeleteLoadBalancer(loadBalancerID string, logicalSwitchName string) error {
	m.deleteCalls = append(m.deleteCalls, struct {
		id  string
		lsw string
	}{id: loadBalancerID, lsw: logicalSwitchName})
	if m.deleteErr != nil {
		return m.deleteErr
	}
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

func TestLoadBalancerControllerAutoNoBackendsStaysProvisioning(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}, deletionDelay: 15}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	// lb-enabled=false 相当: auto 対象が 0 台になる。
	mustCreateLoadBalancerBackendServer(t, database, "web-1", "web-servers", "172.16.10.2", false)

	lb, err := database.CreateLoadBalancer(api.LoadBalancer{
		ApiVersion: "v1",
		Kind:       "LoadBalancer",
		Metadata: api.Metadata{
			Name:     "lb-no-backend",
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.LoadBalancerSpec{
			BackendMode:            util.StringPtr("auto"),
			InternalVirtualNetwork: "web-servers",
			ServerPorts:            []string{"80/tcp"},
			VirtualIpAddress:       util.StringPtr("172.16.10.20"),
		},
	})
	if err != nil {
		t.Fatalf("CreateLoadBalancer() failed: %v", err)
	}

	mockFabric := &mockLoadBalancerFabric{}
	origFactory := newLoadBalancerFabric
	newLoadBalancerFabric = func() networkfabric.LoadBalancerFabric { return mockFabric }
	t.Cleanup(func() {
		newLoadBalancerFabric = origFactory
	})

	ctrl.loadBalancerControllerLoop()
	after, err := database.GetLoadBalancerById(api.LoadBalancerID(lb))
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.LOAD_BALANCER_PROVISIONING {
		t.Fatalf("status = %v, want %d(PROVISIONING)", after.Status, db.LOAD_BALANCER_PROVISIONING)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "no backend servers resolved") {
		t.Fatalf("message = %v, want contains %q", after.Status.Message, "no backend servers resolved")
	}
	if len(mockFabric.ensureCalls) != 0 {
		t.Fatalf("EnsureLoadBalancer() should not be called, got=%d", len(mockFabric.ensureCalls))
	}
}

func TestLoadBalancerControllerManualMissingServerFails(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}, deletionDelay: 15}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")

	lb, err := database.CreateLoadBalancer(api.LoadBalancer{
		ApiVersion: "v1",
		Kind:       "LoadBalancer",
		Metadata: api.Metadata{
			Name:     "lb-manual-missing",
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.LoadBalancerSpec{
			BackendMode:            util.StringPtr("manual"),
			InternalVirtualNetwork: "web-servers",
			InternalServers:        []string{"server-not-found"},
			ServerPorts:            []string{"80/tcp"},
			VirtualIpAddress:       util.StringPtr("172.16.10.30"),
		},
	})
	if err != nil {
		t.Fatalf("CreateLoadBalancer() failed: %v", err)
	}

	ctrl.loadBalancerControllerLoop()
	after, err := database.GetLoadBalancerById(api.LoadBalancerID(lb))
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.LOAD_BALANCER_FAILED {
		t.Fatalf("status = %v, want %d(FAILED)", after.Status, db.LOAD_BALANCER_FAILED)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "manual backend server") {
		t.Fatalf("message = %v, want contains %q", after.Status.Message, "manual backend server")
	}
}

func TestLoadBalancerControllerDeletingCleansDNSAndEntry(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}, deletionDelay: 15}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")

	lb, err := database.CreateLoadBalancer(api.LoadBalancer{
		ApiVersion: "v1",
		Kind:       "LoadBalancer",
		Metadata: api.Metadata{
			Name:     "lb-delete",
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.LoadBalancerSpec{
			BackendMode:            util.StringPtr("auto"),
			InternalVirtualNetwork: "web-servers",
			ServerPorts:            []string{"80/tcp"},
			VirtualIpAddress:       util.StringPtr("172.16.10.40"),
		},
	})
	if err != nil {
		t.Fatalf("CreateLoadBalancer() failed: %v", err)
	}
	lbID := api.LoadBalancerID(lb)

	if err := database.PutDnsEntry("lb-delete", "web-servers", "172.16.10.40"); err != nil {
		t.Fatalf("PutDnsEntry() failed: %v", err)
	}
	if err := database.UpdateLoadBalancerStatusWithMessage(lbID, db.LOAD_BALANCER_DELETING, ""); err != nil {
		t.Fatalf("UpdateLoadBalancerStatusWithMessage() failed: %v", err)
	}
	if err := database.UpdateLoadBalancerById(lbID, api.LoadBalancer{
		Metadata: api.Metadata{Labels: &map[string]interface{}{loadBalancerLabelLogicalSwitch: "marmot-net-web-servers"}},
	}); err != nil {
		t.Fatalf("UpdateLoadBalancerById() failed: %v", err)
	}

	mockFabric := &mockLoadBalancerFabric{}
	origFactory := newLoadBalancerFabric
	newLoadBalancerFabric = func() networkfabric.LoadBalancerFabric { return mockFabric }
	t.Cleanup(func() {
		newLoadBalancerFabric = origFactory
	})

	ctrl.loadBalancerControllerLoop()

	_, err = database.GetLoadBalancerById(lbID)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetLoadBalancerById() err = %v, want ErrNotFound", err)
	}
	if _, err := database.GetDnsEntry("lb-delete", "web-servers"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetDnsEntry() err = %v, want ErrNotFound", err)
	}
	if len(mockFabric.deleteCalls) != 1 {
		t.Fatalf("DeleteLoadBalancer() call count = %d, want 1", len(mockFabric.deleteCalls))
	}
	if mockFabric.deleteCalls[0].id != lbID {
		t.Fatalf("DeleteLoadBalancer() id = %q, want %q", mockFabric.deleteCalls[0].id, lbID)
	}
}

func TestLoadBalancerControllerActiveDriftToProvisioning(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{db: database, marmot: &marmotd.Marmot{NodeName: "hvc", Db: database}, deletionDelay: 15 * time.Second}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-1", "web-servers", "172.16.10.2", true)

	labels := map[string]interface{}{
		db.LoadBalancerLabelAppliedConfig: "outdated-hash",
	}
	lb, err := database.CreateLoadBalancer(api.LoadBalancer{
		ApiVersion: "v1",
		Kind:       "LoadBalancer",
		Metadata: api.Metadata{
			Name:     "lb-drift",
			NodeName: util.StringPtr("hvc"),
			Labels:   &labels,
		},
		Spec: api.LoadBalancerSpec{
			BackendMode:            util.StringPtr("auto"),
			InternalVirtualNetwork: "web-servers",
			ServerPorts:            []string{"80/tcp"},
			VirtualIpAddress:       util.StringPtr("172.16.10.50"),
		},
	})
	if err != nil {
		t.Fatalf("CreateLoadBalancer() failed: %v", err)
	}
	if err := database.UpdateLoadBalancerStatusWithMessage(api.LoadBalancerID(lb), db.LOAD_BALANCER_ACTIVE, ""); err != nil {
		t.Fatalf("UpdateLoadBalancerStatusWithMessage() failed: %v", err)
	}

	ctrl.loadBalancerControllerLoop()
	after, err := database.GetLoadBalancerById(api.LoadBalancerID(lb))
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.LOAD_BALANCER_PROVISIONING {
		t.Fatalf("status = %v, want %d(PROVISIONING)", after.Status, db.LOAD_BALANCER_PROVISIONING)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "configuration drift detected") {
		t.Fatalf("message = %v, want contains %q", after.Status.Message, "configuration drift detected")
	}
}

func TestDesiredLoadBalancerConfigHash_ChangesWhenLogicalSwitchChanges(t *testing.T) {
	lb := api.LoadBalancer{
		Metadata: api.Metadata{Id: "lb-1"},
		Spec: api.LoadBalancerSpec{
			InternalVirtualNetwork: "web-servers",
		},
	}

	h1 := desiredLoadBalancerConfigHash(
		lb,
		"auto",
		"marmot-net-web-servers",
		"172.16.10.50",
		[]string{"80/tcp"},
		[]string{"172.16.10.2"},
	)
	h2 := desiredLoadBalancerConfigHash(
		lb,
		"auto",
		"marmot-net-web-servers-v2",
		"172.16.10.50",
		[]string{"80/tcp"},
		[]string{"172.16.10.2"},
	)

	if h1 == h2 {
		t.Fatalf("config hash should change when logical switch name changes: h1=%q h2=%q", h1, h2)
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
