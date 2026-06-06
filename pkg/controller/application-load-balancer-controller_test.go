package controller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestLoadBalancerControllerStateTransitions(t *testing.T) {
	database := newGatewayTestDatabase(t)
	var desiredHash string
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		if _, err := os.Stat(playbookPath); err != nil {
			return err
		}
		if strings.TrimSpace(targetAddress) == "" || strings.TrimSpace(privateKeyPath) == "" {
			t.Fatalf("ansible runner received empty arguments: target=%q key=%q", targetAddress, privateKeyPath)
		}
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		return applicationLoadBalancerAgentState{LastAppliedHash: desiredHash, LastAppliedAt: time.Now().UTC().Add(time.Hour)}, nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateLoadBalancer(t, database, "lb-unit", "web-servers", "192.168.1.120")

	ctrl.reconcileApplicationLoadBalancerPending(created)

	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending reconcile: %v", err)
	}
	if afterPending.Status == nil || afterPending.Status.StatusCode != db.LOAD_BALANCER_PROVISIONING {
		t.Fatalf("load balancer status after pending reconcile = %v, want %d(PROVISIONING)", afterPending.Status, db.LOAD_BALANCER_PROVISIONING)
	}

	serverID := applicationLoadBalancerManagedServerID(afterPending)
	if serverID == "" {
		t.Fatalf("load balancer managed server id is empty")
	}
	if _, err := database.GetServerById(serverID); err != nil {
		t.Fatalf("GetServerById() failed for managed server %q: %v", serverID, err)
	}

	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")

	beforeProvisioning, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed before provisioning reconcile: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerProvisioning(beforeProvisioning)

	afterProvisioning, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after provisioning reconcile: %v", err)
	}
	if afterProvisioning.Status == nil || afterProvisioning.Status.StatusCode != db.LOAD_BALANCER_CONFIGURING {
		t.Fatalf("load balancer status after provisioning reconcile = %v, want %d(CONFIGURING)", afterProvisioning.Status, db.LOAD_BALANCER_CONFIGURING)
	}

	listenerBackends, err := ctrl.resolveApplicationLoadBalancerListenerBackends(afterProvisioning)
	if err != nil {
		t.Fatalf("resolveApplicationLoadBalancerListenerBackends() failed: %v", err)
	}
	desiredHash, err = desiredApplicationLoadBalancerConfigHash(afterProvisioning, listenerBackends)
	if err != nil {
		t.Fatalf("desiredApplicationLoadBalancerConfigHash() failed: %v", err)
	}

	ctrl.reconcileApplicationLoadBalancerConfiguring(afterProvisioning)

	afterConfiguring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after configuring reconcile: %v", err)
	}
	if afterConfiguring.Status == nil || afterConfiguring.Status.StatusCode != db.LOAD_BALANCER_ACTIVE {
		t.Fatalf("load balancer status after configuring reconcile = %v, want %d(ACTIVE)", afterConfiguring.Status, db.LOAD_BALANCER_ACTIVE)
	}
}

func TestLoadBalancerControllerWaitsForAgentApply(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		return applicationLoadBalancerAgentState{}, nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateLoadBalancer(t, database, "lb-wait", "web-servers", "192.168.1.124")

	ctrl.reconcileApplicationLoadBalancerPending(created)
	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending reconcile: %v", err)
	}
	serverID := applicationLoadBalancerManagedServerID(afterPending)
	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")
	ctrl.reconcileApplicationLoadBalancerProvisioning(afterPending)

	configuring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after provisioning reconcile: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerConfiguring(configuring)

	afterConfiguring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after configuring reconcile: %v", err)
	}
	if afterConfiguring.Status == nil || afterConfiguring.Status.StatusCode != db.LOAD_BALANCER_CONFIGURING {
		t.Fatalf("load balancer status after waiting for agent = %v, want %d(CONFIGURING)", afterConfiguring.Status, db.LOAD_BALANCER_CONFIGURING)
	}
	if got := applicationLoadBalancerAppliedConfigHash(afterConfiguring); got != "" {
		t.Fatalf("applied hash should stay empty until agent confirms apply, got %q", got)
	}
	if got := applicationLoadBalancerStagedConfigHash(afterConfiguring); got == "" {
		t.Fatalf("staged hash should be set after playbook deployment")
	}
}

func TestLoadBalancerControllerWaitsForNewerAgentApplyResult(t *testing.T) {
	database := newGatewayTestDatabase(t)
	var desiredHash string
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		return applicationLoadBalancerAgentState{LastAppliedHash: desiredHash, LastAppliedAt: time.Time{}}, nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateLoadBalancer(t, database, "lb-stale", "web-servers", "192.168.1.125")

	ctrl.reconcileApplicationLoadBalancerPending(created)
	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending reconcile: %v", err)
	}
	serverID := applicationLoadBalancerManagedServerID(afterPending)
	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")
	ctrl.reconcileApplicationLoadBalancerProvisioning(afterPending)

	configuring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after provisioning reconcile: %v", err)
	}
	listenerBackends, err := ctrl.resolveApplicationLoadBalancerListenerBackends(configuring)
	if err != nil {
		t.Fatalf("resolveApplicationLoadBalancerListenerBackends() failed: %v", err)
	}
	desiredHash, err = desiredApplicationLoadBalancerConfigHash(configuring, listenerBackends)
	if err != nil {
		t.Fatalf("desiredApplicationLoadBalancerConfigHash() failed: %v", err)
	}

	ctrl.reconcileApplicationLoadBalancerConfiguring(configuring)

	afterConfiguring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after configuring reconcile: %v", err)
	}
	if afterConfiguring.Status == nil || afterConfiguring.Status.StatusCode != db.LOAD_BALANCER_CONFIGURING {
		t.Fatalf("load balancer status with stale agent timestamp = %v, want %d(CONFIGURING)", afterConfiguring.Status, db.LOAD_BALANCER_CONFIGURING)
	}
}

func TestLoadBalancerControllerAgentStateReadFailureThreshold(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		return applicationLoadBalancerAgentState{}, errors.New("agent state read failed")
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateLoadBalancer(t, database, "lb-agent-failure", "web-servers", "192.168.1.126")

	ctrl.reconcileApplicationLoadBalancerPending(created)
	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending reconcile: %v", err)
	}
	serverID := applicationLoadBalancerManagedServerID(afterPending)
	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")
	ctrl.reconcileApplicationLoadBalancerProvisioning(afterPending)

	for i := 0; i < applicationLoadBalancerAgentStateReadMaxFailures; i++ {
		configuring, err := database.GetLoadBalancerById(lbID)
		if err != nil {
			t.Fatalf("GetLoadBalancerById() failed before configuring reconcile: %v", err)
		}
		ctrl.reconcileApplicationLoadBalancerConfiguring(configuring)
	}

	after, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after repeated read failures: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.LOAD_BALANCER_DEGRADED {
		t.Fatalf("load balancer status after repeated agent read failures = %v, want %d(DEGRADED)", after.Status, db.LOAD_BALANCER_DEGRADED)
	}
	if after.Metadata.Labels == nil {
		t.Fatalf("labels should be present")
	}
	if got := db.GetLoadBalancerAgentStateReadFailures(*after.Metadata.Labels); got != applicationLoadBalancerAgentStateReadMaxFailures {
		t.Fatalf("agent state read failures = %d, want %d", got, applicationLoadBalancerAgentStateReadMaxFailures)
	}
}

func TestLoadBalancerControllerRecoversAfterConsecutiveAgentReadSuccesses(t *testing.T) {
	database := newGatewayTestDatabase(t)
	var failReads bool
	var desiredHash string
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		if failReads {
			return applicationLoadBalancerAgentState{}, errors.New("agent state read failed")
		}
		return applicationLoadBalancerAgentState{LastAppliedHash: desiredHash, LastAppliedAt: time.Now().UTC().Add(time.Hour)}, nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateLoadBalancer(t, database, "lb-recover", "web-servers", "192.168.1.127")

	ctrl.reconcileApplicationLoadBalancerPending(created)
	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending reconcile: %v", err)
	}
	serverID := applicationLoadBalancerManagedServerID(afterPending)
	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")
	ctrl.reconcileApplicationLoadBalancerProvisioning(afterPending)

	configuring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after provisioning reconcile: %v", err)
	}
	listenerBackends, err := ctrl.resolveApplicationLoadBalancerListenerBackends(configuring)
	if err != nil {
		t.Fatalf("resolveApplicationLoadBalancerListenerBackends() failed: %v", err)
	}
	desiredHash, err = desiredApplicationLoadBalancerConfigHash(configuring, listenerBackends)
	if err != nil {
		t.Fatalf("desiredApplicationLoadBalancerConfigHash() failed: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerConfiguring(configuring)

	active, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after initial configuring reconcile: %v", err)
	}
	if active.Status == nil || active.Status.StatusCode != db.LOAD_BALANCER_ACTIVE {
		t.Fatalf("load balancer status after initial activation = %v, want %d(ACTIVE)", active.Status, db.LOAD_BALANCER_ACTIVE)
	}

	failReads = true
	for i := 0; i < applicationLoadBalancerAgentStateReadMaxFailures; i++ {
		current, err := database.GetLoadBalancerById(lbID)
		if err != nil {
			t.Fatalf("GetLoadBalancerById() failed while forcing degraded: %v", err)
		}
		ctrl.reconcileApplicationLoadBalancerActive(current)
	}

	degraded, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after forcing degraded: %v", err)
	}
	if degraded.Status == nil || degraded.Status.StatusCode != db.LOAD_BALANCER_DEGRADED {
		t.Fatalf("load balancer status after forcing degraded = %v, want %d(DEGRADED)", degraded.Status, db.LOAD_BALANCER_DEGRADED)
	}

	failReads = false
	firstRecovery, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed before first recovery reconcile: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerActive(firstRecovery)

	afterFirstRecovery, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after first recovery reconcile: %v", err)
	}
	if afterFirstRecovery.Status == nil || afterFirstRecovery.Status.StatusCode != db.LOAD_BALANCER_DEGRADED {
		t.Fatalf("load balancer status after first recovery reconcile = %v, want %d(DEGRADED)", afterFirstRecovery.Status, db.LOAD_BALANCER_DEGRADED)
	}

	secondRecovery, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed before second recovery reconcile: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerActive(secondRecovery)

	afterSecondRecovery, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after second recovery reconcile: %v", err)
	}
	if afterSecondRecovery.Status == nil || afterSecondRecovery.Status.StatusCode != db.LOAD_BALANCER_ACTIVE {
		t.Fatalf("load balancer status after second recovery reconcile = %v, want %d(ACTIVE)", afterSecondRecovery.Status, db.LOAD_BALANCER_ACTIVE)
	}
}

func TestLoadBalancerControllerDeletingRemovesObject(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		return applicationLoadBalancerAgentState{}, nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateLoadBalancer(t, database, "lb-del", "web-servers", "192.168.1.121")
	ctrl.reconcileApplicationLoadBalancerPending(created)

	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed: %v", err)
	}
	serverID := applicationLoadBalancerManagedServerID(afterPending)
	if serverID == "" {
		t.Fatalf("load balancer managed server id is empty")
	}
	desiredConfigPath := applicationLoadBalancerDesiredConfigPath(lbID)
	if err := os.MkdirAll(filepath.Dir(desiredConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() failed for desired config directory: %v", err)
	}
	if err := os.WriteFile(desiredConfigPath, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed for desired config: %v", err)
	}

	if err := database.SetDeleteTimestampLoadBalancer(lbID); err != nil {
		t.Fatalf("SetDeleteTimestampLoadBalancer() failed: %v", err)
	}

	deleting, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed in deleting phase: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerDeleting(deleting)

	if err := database.DeleteServerById(serverID); err != nil {
		t.Fatalf("DeleteServerById() failed: %v", err)
	}

	deleting, err = database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed before final delete reconcile: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerDeleting(deleting)

	_, err = database.GetLoadBalancerById(lbID)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("GetLoadBalancerById() error = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(desiredConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("desired config file still exists after delete reconcile: %v", err)
	}
}

func TestLoadBalancerControllerDegradedRecoveryByBackendMatch(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupApplicationLoadBalancerAnsibleTestHooks(t, func(playbookPath, targetAddress, privateKeyPath string) error {
		return nil
	}, func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error) {
		return applicationLoadBalancerAgentState{}, nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	created := mustCreateLoadBalancer(t, database, "lb-degraded", "web-servers", "192.168.1.122")

	ctrl.reconcileApplicationLoadBalancerPending(created)
	lbID := api.LoadBalancerID(created)
	afterPending, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after pending reconcile: %v", err)
	}
	serverID := applicationLoadBalancerManagedServerID(afterPending)
	database.UpdateServerStatus(serverID, db.SERVER_RUNNING, "")

	ctrl.reconcileApplicationLoadBalancerProvisioning(afterPending)
	configuring, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after provisioning reconcile: %v", err)
	}
	ctrl.reconcileApplicationLoadBalancerConfiguring(configuring)

	degraded, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after configuring reconcile: %v", err)
	}
	if degraded.Status == nil || degraded.Status.StatusCode != db.LOAD_BALANCER_DEGRADED {
		t.Fatalf("load balancer status after configuring without backend = %v, want %d(DEGRADED)", degraded.Status, db.LOAD_BALANCER_DEGRADED)
	}

	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	ctrl.reconcileApplicationLoadBalancerActive(degraded)

	afterRecoveryCheck, err := database.GetLoadBalancerById(lbID)
	if err != nil {
		t.Fatalf("GetLoadBalancerById() failed after recovery check: %v", err)
	}
	if afterRecoveryCheck.Status == nil || afterRecoveryCheck.Status.StatusCode != db.LOAD_BALANCER_CONFIGURING {
		t.Fatalf("load balancer status after backend recovery check = %v, want %d(CONFIGURING)", afterRecoveryCheck.Status, db.LOAD_BALANCER_CONFIGURING)
	}
}

func mustCreateLoadBalancer(t *testing.T, database *db.Database, name, internalNetwork, publicIP string) api.ApplicationLoadBalancer {
	t.Helper()
	item, err := database.CreateLoadBalancer(api.ApplicationLoadBalancer{
		ApiVersion: "v1",
		Kind:       "ApplicationLoadBalancer",
		Metadata: api.Metadata{
			Name:     name,
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.LoadBalancerSpec{
			BindPublicIpAddress:    publicIP,
			InternalVirtualNetwork: internalNetwork,
			RemoteCIDR:             "0.0.0.0/0",
			Listeners: []api.LoadBalancerListener{{
				Name:        "http",
				Protocol:    "HTTP",
				VipPort:     80,
				BackendPort: 8080,
				BackendSelector: api.LoadBalancerLabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
				LoadBalancingAlgorithm: "roundrobin",
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateLoadBalancer() failed: %v", err)
	}
	if api.LoadBalancerID(item) == "" {
		t.Fatalf("CreateLoadBalancer() returned empty load balancer id")
	}
	return item
}

func mustCreateLoadBalancerBackendServer(t *testing.T, database *db.Database, name, networkName, ipAddress string) api.Server {
	t.Helper()
	labels := map[string]interface{}{"app": "web"}
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
		t.Fatalf("MakeServerEntry() failed for load balancer backend server: %v", err)
	}
	database.UpdateServerStatus(api.ServerID(server), db.SERVER_RUNNING, "")
	return server
}

func setupApplicationLoadBalancerAnsibleTestHooks(t *testing.T, runner func(playbookPath, targetAddress, privateKeyPath string) error, stateReader func(targetAddress, privateKeyPath string) (applicationLoadBalancerAgentState, error)) {
	t.Helper()
	oldRunner := runApplicationLoadBalancerPlaybook
	oldDir := applicationLoadBalancerPlaybookDir
	oldDesiredDir := applicationLoadBalancerDesiredDir
	oldKey := applicationLoadBalancerPrivateKeyPath
	oldStateReader := readApplicationLoadBalancerAgentState
	oldDesiredHashReader := readApplicationLoadBalancerDesiredConfigHash

	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "private.key")
	if err := os.WriteFile(keyPath, []byte("dummy-private-key"), 0o600); err != nil {
		t.Fatalf("WriteFile() failed for test private key: %v", err)
	}

	runApplicationLoadBalancerPlaybook = runner
	applicationLoadBalancerPlaybookDir = filepath.Join(tempDir, "playbooks")
	applicationLoadBalancerDesiredDir = filepath.Join(tempDir, "desired-configs")
	applicationLoadBalancerPrivateKeyPath = keyPath
	readApplicationLoadBalancerAgentState = stateReader
	readApplicationLoadBalancerDesiredConfigHash = func(targetAddress, privateKeyPath, desiredConfigPath string) (string, error) {
		return fileSHA256Hex(desiredConfigPath)
	}

	t.Cleanup(func() {
		runApplicationLoadBalancerPlaybook = oldRunner
		applicationLoadBalancerPlaybookDir = oldDir
		applicationLoadBalancerDesiredDir = oldDesiredDir
		applicationLoadBalancerPrivateKeyPath = oldKey
		readApplicationLoadBalancerAgentState = oldStateReader
		readApplicationLoadBalancerDesiredConfigHash = oldDesiredHashReader
	})
}

func TestLoadBalancerControllerSettingsFromEnv(t *testing.T) {
	oldInterval := applicationLoadBalancerControllerInterval
	oldMaxFailures := applicationLoadBalancerAgentStateReadMaxFailures
	oldRecovery := applicationLoadBalancerAgentStateRecoverySuccessRequired
	t.Cleanup(func() {
		applicationLoadBalancerControllerInterval = oldInterval
		applicationLoadBalancerAgentStateReadMaxFailures = oldMaxFailures
		applicationLoadBalancerAgentStateRecoverySuccessRequired = oldRecovery
	})

	t.Setenv("MARMOT_LB_CONTROLLER_INTERVAL_SECONDS", "9")
	t.Setenv("MARMOT_LB_AGENT_STATE_READ_MAX_FAILURES", "5")
	t.Setenv("MARMOT_LB_AGENT_RECOVERY_SUCCESS_REQUIRED", "4")

	applicationLoadBalancerControllerSettingsFromEnv()

	if applicationLoadBalancerControllerInterval != 9*time.Second {
		t.Fatalf("applicationLoadBalancerControllerInterval = %v, want %v", applicationLoadBalancerControllerInterval, 9*time.Second)
	}
	if applicationLoadBalancerAgentStateReadMaxFailures != 5 {
		t.Fatalf("applicationLoadBalancerAgentStateReadMaxFailures = %d, want 5", applicationLoadBalancerAgentStateReadMaxFailures)
	}
	if applicationLoadBalancerAgentStateRecoverySuccessRequired != 4 {
		t.Fatalf("applicationLoadBalancerAgentStateRecoverySuccessRequired = %d, want 4", applicationLoadBalancerAgentStateRecoverySuccessRequired)
	}
}
