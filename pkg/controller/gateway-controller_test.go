package controller

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestGatewayControllerStateTransitions(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	createdGateway := mustCreateGateway(t, database, "igw-unit", "web-servers", "192.168.1.110")

	ctrl.reconcileGatewayPending(createdGateway)

	gatewayID := api.GatewayID(createdGateway)
	gatewayAfterPending, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after pending reconcile: %v", err)
	}
	if gatewayAfterPending.Status == nil || gatewayAfterPending.Status.StatusCode != db.GATEWAY_PROVISIONING {
		t.Fatalf("gateway status after pending reconcile = %v, want %d(PROVISIONING)", gatewayAfterPending.Status, db.GATEWAY_PROVISIONING)
	}

	serverID := gatewayManagedServerID(gatewayAfterPending)
	if strings.TrimSpace(serverID) == "" {
		t.Fatalf("gateway managed server id is empty")
	}
	if _, err := database.GetServerById(serverID); err != nil {
		t.Fatalf("GetServerById() failed for managed server %q: %v", serverID, err)
	}

	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")

	gatewayBeforeProvisioning, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed before provisioning reconcile: %v", err)
	}
	ctrl.reconcileGatewayProvisioning(gatewayBeforeProvisioning)

	gatewayAfterProvisioning, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after provisioning reconcile: %v", err)
	}
	if gatewayAfterProvisioning.Status == nil || gatewayAfterProvisioning.Status.StatusCode != db.GATEWAY_ACTIVE {
		t.Fatalf("gateway status after provisioning reconcile = %v, want %d(ACTIVE)", gatewayAfterProvisioning.Status, db.GATEWAY_ACTIVE)
	}
}

func TestGatewayControllerLoopIntegration_CreateToActive(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	createdGateway := mustCreateGateway(t, database, "igw-int", "web-servers", "192.168.1.111")
	gatewayID := api.GatewayID(createdGateway)

	ctrl.gatewayControllerLoop()

	gatewayAfterFirstLoop, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after first loop: %v", err)
	}
	if gatewayAfterFirstLoop.Status == nil || gatewayAfterFirstLoop.Status.StatusCode != db.GATEWAY_PROVISIONING {
		t.Fatalf("gateway status after first loop = %v, want %d(PROVISIONING)", gatewayAfterFirstLoop.Status, db.GATEWAY_PROVISIONING)
	}

	serverID := gatewayManagedServerID(gatewayAfterFirstLoop)
	if strings.TrimSpace(serverID) == "" {
		t.Fatalf("gateway managed server id is empty after first loop")
	}

	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")
	ctrl.gatewayControllerLoop()

	gatewayAfterSecondLoop, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after second loop: %v", err)
	}
	if gatewayAfterSecondLoop.Status == nil || gatewayAfterSecondLoop.Status.StatusCode != db.GATEWAY_ACTIVE {
		t.Fatalf("gateway status after second loop = %v, want %d(ACTIVE)", gatewayAfterSecondLoop.Status, db.GATEWAY_ACTIVE)
	}
}

func mustCreateVirtualNetwork(t *testing.T, database *db.Database, name string) api.VirtualNetwork {
	t.Helper()
	vnet, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata: api.Metadata{
			Name: name,
		},
		Spec: api.VirtualNetworkSpec{},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork() failed: %v", err)
	}
	return vnet
}

func mustCreateGateway(t *testing.T, database *db.Database, name, internalNetwork, publicIP string) api.Gateway {
	t.Helper()
	gateway, err := database.CreateGateway(api.Gateway{
		ApiVersion: "v1",
		Kind:       "Gateway",
		Metadata: api.Metadata{
			Name:     name,
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.GatewaySpec{
			BindPublicIpAddress:    publicIP,
			InternalServerName:     "server-10",
			InternalVirtualNetwork: internalNetwork,
			ServerPorts:            []string{"80/tcp"},
		},
	})
	if err != nil {
		t.Fatalf("CreateGateway() failed: %v", err)
	}
	if strings.TrimSpace(api.GatewayID(gateway)) == "" {
		t.Fatalf("CreateGateway() returned empty gateway id")
	}
	return gateway
}

func newGatewayTestDatabase(t *testing.T) *db.Database {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("MARMOT_TEST_ETCD_ENDPOINT"))
	if endpoint == "" {
		endpoint = startGatewayTestEtcdContainer(t)
	}

	database, err := db.NewDatabase(endpoint)
	if err != nil {
		t.Fatalf("NewDatabase(%q) failed: %v", endpoint, err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

func startGatewayTestEtcdContainer(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker command not found; set MARMOT_TEST_ETCD_ENDPOINT to run gateway controller tests without docker")
	}

	port := getFreeTCPPort(t)
	portMapping := fmt.Sprintf("%d:2379", port)
	cmd := exec.Command("docker", "run", "-d", "--rm", "-p", portMapping, "ghcr.io/takara9/etcd:3.6.5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("failed to start etcd test container: %v, output=%s", err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	t.Cleanup(func() {
		stopCmd := exec.Command("docker", "stop", containerID)
		_, _ = stopCmd.CombinedOutput()
	})

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		database, err := db.NewDatabase(endpoint)
		if err == nil {
			_ = database.Close()
			return endpoint
		}
		time.Sleep(300 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for etcd container at %s", endpoint)
	return ""
}

func getFreeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() failed while reserving test port: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is not TCP")
	}
	return addr.Port
}
