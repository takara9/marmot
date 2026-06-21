package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
	"golang.org/x/crypto/ssh"
)

func TestReadGatewayPublicKeyIfExists_ReadsPublicKeyFile(t *testing.T) {
	oldPublicKeyPath := gatewayPublicKeyPath
	t.Cleanup(func() {
		gatewayPublicKeyPath = oldPublicKeyPath
	})

	tempDir := t.TempDir()
	gatewayPublicKeyPath = filepath.Join(tempDir, "public.key")

	want := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDmarmot-test-key marmot-gateway"
	if err := os.WriteFile(gatewayPublicKeyPath, []byte(want+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed for public key: %v", err)
	}

	got := readGatewayPublicKeyIfExists()
	if got != want {
		t.Fatalf("readGatewayPublicKeyIfExists() = %q, want %q", got, want)
	}
}

func TestReadGatewayPublicKeyIfExists_ReturnsEmptyWhenPublicKeyFileMissing(t *testing.T) {
	oldPublicKeyPath := gatewayPublicKeyPath
	t.Cleanup(func() {
		gatewayPublicKeyPath = oldPublicKeyPath
	})

	tempDir := t.TempDir()
	gatewayPublicKeyPath = filepath.Join(tempDir, "public.key")

	got := readGatewayPublicKeyIfExists()
	if got != "" {
		t.Fatalf("readGatewayPublicKeyIfExists() = %q, want empty", got)
	}
}

func TestGatewayControllerStateTransitions(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupGatewayAnsibleTestHooks(t, func(playbookPath, gatewayAddress, privateKeyPath string) error {
		if _, err := os.Stat(playbookPath); err != nil {
			return err
		}
		if strings.TrimSpace(gatewayAddress) == "" || strings.TrimSpace(privateKeyPath) == "" {
			t.Fatalf("ansible runner received empty arguments: gateway=%q key=%q", gatewayAddress, privateKeyPath)
		}
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateInternalServer(t, database, "server-10", "web-servers", "172.16.10.2")
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
	if gatewayAfterProvisioning.Status == nil || gatewayAfterProvisioning.Status.StatusCode != db.GATEWAY_WAITING_READY {
		t.Fatalf("gateway status after provisioning reconcile = %v, want %d(WAITING_READY)", gatewayAfterProvisioning.Status, db.GATEWAY_WAITING_READY)
	}

	ctrl.reconcileGatewayWaitOSUp(gatewayAfterProvisioning)
	gatewayAfterWaitOSUpFirst, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after first wait-os-up reconcile: %v", err)
	}
	if gatewayAfterWaitOSUpFirst.Status == nil || gatewayAfterWaitOSUpFirst.Status.StatusCode != db.GATEWAY_WAITING_READY {
		t.Fatalf("gateway status after first wait-os-up reconcile = %v, want %d(WAITING_READY)", gatewayAfterWaitOSUpFirst.Status, db.GATEWAY_WAITING_READY)
	}
	if err := ctrl.updateGatewayLabels(gatewayID, func(labels map[string]interface{}) {
		db.SetGatewayWaitOSUpReadyAt(labels, time.Now().Add(-1*time.Second).Unix())
	}); err != nil {
		t.Fatalf("updateGatewayLabels() failed for wait-os-up readiness: %v", err)
	}
	gatewayBeforeWaitOSUpSecond, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed before second wait-os-up reconcile: %v", err)
	}
	ctrl.reconcileGatewayWaitOSUp(gatewayBeforeWaitOSUpSecond)
	gatewayAfterWaitOSUpSecond, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after second wait-os-up reconcile: %v", err)
	}
	if gatewayAfterWaitOSUpSecond.Status == nil || gatewayAfterWaitOSUpSecond.Status.StatusCode != db.GATEWAY_CONFIGURING {
		t.Fatalf("gateway status after second wait-os-up reconcile = %v, want %d(CONFIGURING)", gatewayAfterWaitOSUpSecond.Status, db.GATEWAY_CONFIGURING)
	}

	ctrl.reconcileGatewayConfiguring(gatewayAfterWaitOSUpSecond)

	gatewayAfterConfiguring, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after configuring reconcile: %v", err)
	}
	if gatewayAfterConfiguring.Status == nil || gatewayAfterConfiguring.Status.StatusCode != db.GATEWAY_ACTIVE {
		t.Fatalf("gateway status after configuring reconcile = %v, want %d(ACTIVE)", gatewayAfterConfiguring.Status, db.GATEWAY_ACTIVE)
	}
	if gatewayAfterConfiguring.Status.Message != nil && strings.TrimSpace(*gatewayAfterConfiguring.Status.Message) != "" {
		t.Fatalf("gateway message after configuring reconcile = %q, want empty", *gatewayAfterConfiguring.Status.Message)
	}
}

func TestGatewayControllerLoopIntegration_CreateToActive(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupGatewayAnsibleTestHooks(t, func(playbookPath, gatewayAddress, privateKeyPath string) error {
		if _, err := os.Stat(playbookPath); err != nil {
			return err
		}
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateInternalServer(t, database, "server-10", "web-servers", "172.16.10.2")
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
	if gatewayAfterSecondLoop.Status == nil || gatewayAfterSecondLoop.Status.StatusCode != db.GATEWAY_WAITING_READY {
		t.Fatalf("gateway status after second loop = %v, want %d(WAITING_READY)", gatewayAfterSecondLoop.Status, db.GATEWAY_WAITING_READY)
	}

	if err := ctrl.updateGatewayLabels(gatewayID, func(labels map[string]interface{}) {
		db.SetGatewayWaitOSUpReadyAt(labels, time.Now().Add(-1*time.Second).Unix())
	}); err != nil {
		t.Fatalf("updateGatewayLabels() failed for integration wait-os-up readiness: %v", err)
	}
	ctrl.gatewayControllerLoop()

	gatewayAfterThirdLoop, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after third loop: %v", err)
	}
	if gatewayAfterThirdLoop.Status == nil || gatewayAfterThirdLoop.Status.StatusCode != db.GATEWAY_CONFIGURING {
		t.Fatalf("gateway status after third loop = %v, want %d(CONFIGURING)", gatewayAfterThirdLoop.Status, db.GATEWAY_CONFIGURING)
	}

	ctrl.gatewayControllerLoop()

	gatewayAfterFourthLoop, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after fourth loop: %v", err)
	}
	if gatewayAfterFourthLoop.Status == nil || gatewayAfterFourthLoop.Status.StatusCode != db.GATEWAY_ACTIVE {
		t.Fatalf("gateway status after fourth loop = %v, want %d(ACTIVE)", gatewayAfterFourthLoop.Status, db.GATEWAY_ACTIVE)
	}
	if gatewayAfterFourthLoop.Status.Message != nil && strings.TrimSpace(*gatewayAfterFourthLoop.Status.Message) != "" {
		t.Fatalf("gateway message after fourth loop = %q, want empty", *gatewayAfterFourthLoop.Status.Message)
	}
}

func TestGatewayControllerConfigRetryExceeded(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupGatewayAnsibleTestHooks(t, func(playbookPath, gatewayAddress, privateKeyPath string) error {
		return fmt.Errorf("simulated ansible failure")
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateInternalServer(t, database, "server-10", "web-servers", "172.16.10.2")
	createdGateway := mustCreateGateway(t, database, "igw-retry", "web-servers", "192.168.1.112")
	gatewayID := api.GatewayID(createdGateway)

	ctrl.reconcileGatewayPending(createdGateway)
	gatewayAfterPending, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after pending reconcile: %v", err)
	}
	serverID := gatewayManagedServerID(gatewayAfterPending)
	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")
	ctrl.reconcileGatewayProvisioning(gatewayAfterPending)

	for i := 0; i < gatewayAnsibleMaxRetryCount; i++ {
		gateway, err := database.GetGatewayById(gatewayID)
		if err != nil {
			t.Fatalf("GetGatewayById() failed in retry loop: %v", err)
		}
		ctrl.reconcileGatewayConfiguring(gateway)
	}

	failedGateway, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after retry exhaustion: %v", err)
	}
	if failedGateway.Status == nil || failedGateway.Status.StatusCode != db.GATEWAY_FAILED {
		t.Fatalf("gateway status after retry exhaustion = %v, want %d(FAILED)", failedGateway.Status, db.GATEWAY_FAILED)
	}
	if failedGateway.Metadata.Labels == nil {
		t.Fatalf("gateway labels missing after retry exhaustion")
	}
	if got := db.GetGatewayAnsibleRetries(*failedGateway.Metadata.Labels); got != gatewayAnsibleMaxRetryCount {
		t.Fatalf("ansible retries = %d, want %d", got, gatewayAnsibleMaxRetryCount)
	}
}

func TestGatewayControllerDeletesGatewayWhenInternalServerMissing(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupGatewayAnsibleTestHooks(t, func(playbookPath, gatewayAddress, privateKeyPath string) error {
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	internalServer := mustCreateInternalServer(t, database, "server-10", "web-servers", "172.16.10.2")
	createdGateway := mustCreateGateway(t, database, "igw-missing-target", "web-servers", "192.168.1.113")

	ctrl.reconcileGatewayPending(createdGateway)

	if err := database.DeleteServerById(api.ServerID(internalServer)); err != nil {
		t.Fatalf("DeleteServerById() failed for internal server: %v", err)
	}

	ctrl.gatewayControllerLoop()

	_, err := database.GetGatewayById(api.GatewayID(createdGateway))
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetGatewayById() error = %v, want ErrNotFound", err)
	}
}

func TestGatewayControllerFailedStateRecoversWhenManagedServerMissing(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupGatewayAnsibleTestHooks(t, func(playbookPath, gatewayAddress, privateKeyPath string) error {
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateInternalServer(t, database, "server-10", "web-servers", "172.16.10.2")
	createdGateway := mustCreateGateway(t, database, "igw-failed-recover", "web-servers", "192.168.1.114")
	gatewayID := api.GatewayID(createdGateway)

	failedGateway, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed: %v", err)
	}
	if failedGateway.Metadata.Labels == nil {
		labels := map[string]interface{}{}
		failedGateway.Metadata.Labels = &labels
	}
	labels := *failedGateway.Metadata.Labels
	db.SetGatewayManagedServerID(labels, "missing-server")
	db.SetGatewayAnsibleRetries(labels, 3)
	failedGateway.Metadata.Labels = &labels
	now := time.Now()
	failedGateway.Status = &api.Status{
		StatusCode:          db.GATEWAY_FAILED,
		Status:              util.StringPtr(db.GatewayStatus[db.GATEWAY_FAILED]),
		Message:             util.StringPtr("gateway ansible apply failed (3/3): simulated"),
		CreationTimeStamp:   util.TimePtr(now),
		LastUpdateTimeStamp: util.TimePtr(now),
	}
	if err := database.UpdateGatewayById(gatewayID, failedGateway); err != nil {
		t.Fatalf("UpdateGatewayById() failed: %v", err)
	}

	ctrl.gatewayControllerLoop()

	afterRecovery, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after recovery loop: %v", err)
	}
	if afterRecovery.Status == nil || afterRecovery.Status.StatusCode != db.GATEWAY_PENDING {
		t.Fatalf("gateway status after failed recovery = %v, want %d(PENDING)", afterRecovery.Status, db.GATEWAY_PENDING)
	}
	if afterRecovery.Metadata.Labels != nil {
		if got := db.GetGatewayManagedServerID(*afterRecovery.Metadata.Labels); got != "" {
			t.Fatalf("gateway managed server id after recovery = %q, want empty", got)
		}
		if got := db.GetGatewayAnsibleRetries(*afterRecovery.Metadata.Labels); got != 0 {
			t.Fatalf("gateway ansible retries after recovery = %d, want 0", got)
		}
	}

	ctrl.gatewayControllerLoop()

	afterRecreate, err := database.GetGatewayById(gatewayID)
	if err != nil {
		t.Fatalf("GetGatewayById() failed after recreate loop: %v", err)
	}
	if afterRecreate.Status == nil || afterRecreate.Status.StatusCode != db.GATEWAY_PROVISIONING {
		t.Fatalf("gateway status after recreate loop = %v, want %d(PROVISIONING)", afterRecreate.Status, db.GATEWAY_PROVISIONING)
	}
	if afterRecreate.Metadata.Labels == nil {
		t.Fatalf("gateway labels missing after recreate loop")
	}
	if got := db.GetGatewayManagedServerID(*afterRecreate.Metadata.Labels); strings.TrimSpace(got) == "" {
		t.Fatalf("gateway managed server id after recreate loop is empty")
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

func mustCreateInternalServer(t *testing.T, database *db.Database, name, networkName, ipAddress string) api.Server {
	t.Helper()
	server, err := database.MakeServerEntry(api.Server{
		ApiVersion: "v1",
		Kind:       "Server",
		Metadata: api.Metadata{
			Name:     name,
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.ServerSpec{
			NetworkInterface: &[]api.NetworkInterface{{
				Networkname: networkName,
				Address:     util.StringPtr(ipAddress),
			}},
		},
	})
	if err != nil {
		t.Fatalf("MakeServerEntry() failed for internal server: %v", err)
	}
	database.UpdateServerStatus(api.ServerID(server), db.SERVER_RUNNING, "")
	return server
}

func setupGatewayAnsibleTestHooks(t *testing.T, runner func(playbookPath, gatewayAddress, privateKeyPath string) error) {
	t.Helper()
	oldRunner := runGatewayPlaybook
	oldPing := runGatewayPing
	oldBecomeCheck := runGatewayBecomeCheck
	oldDir := gatewayPlaybookDir
	oldKey := gatewayPrivateKeyPath
	oldPublic := gatewayPublicKeyPath

	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "private.key")
	publicKeyPath := filepath.Join(tempDir, "public.key")
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() failed for test private key: %v", err)
	}
	privateBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(privateBlock), 0o600); err != nil {
		t.Fatalf("WriteFile() failed for test private key: %v", err)
	}
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey() failed for test private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, ssh.MarshalAuthorizedKey(publicKey), 0o644); err != nil {
		t.Fatalf("WriteFile() failed for test public key: %v", err)
	}

	runGatewayPlaybook = runner
	runGatewayPing = func(address, privateKeyPath string) error { return nil }
	runGatewayBecomeCheck = func(address, privateKeyPath string) error { return nil }
	gatewayPlaybookDir = filepath.Join(tempDir, "playbooks")
	gatewayPrivateKeyPath = keyPath
	gatewayPublicKeyPath = publicKeyPath

	t.Cleanup(func() {
		runGatewayPlaybook = oldRunner
		runGatewayPing = oldPing
		runGatewayBecomeCheck = oldBecomeCheck
		gatewayPlaybookDir = oldDir
		gatewayPrivateKeyPath = oldKey
		gatewayPublicKeyPath = oldPublic
	})
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

func TestFirstHostAddressFromCIDR(t *testing.T) {
	t.Run("ipv4", func(t *testing.T) {
		got, err := firstHostAddressFromCIDR("172.16.30.0/24")
		if err != nil {
			t.Fatalf("firstHostAddressFromCIDR() failed: %v", err)
		}
		if got != "172.16.30.1" {
			t.Fatalf("firstHostAddressFromCIDR() = %q, want %q", got, "172.16.30.1")
		}
	})

	t.Run("ipv6 rejected", func(t *testing.T) {
		if _, err := firstHostAddressFromCIDR("2001:db8::/64"); err == nil {
			t.Fatalf("firstHostAddressFromCIDR() expected error for IPv6 CIDR")
		}
	})
}
