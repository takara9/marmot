package controller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestNetworkLoadBalancerControllerFailsWhenNoBackendMatched(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupNetworkLoadBalancerTestHooks(t, func(script string) error { return nil })
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-no-backend", "web-servers", "192.168.1.180")

	ctrl.reconcileNetworkLoadBalancerDesiredState(created)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_FAILED {
		t.Fatalf("network load balancer status = %v, want %d(FAILED)", after.Status, db.NETWORK_LOAD_BALANCER_FAILED)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "no backend matched") {
		t.Fatalf("status message = %v, want no backend matched", after.Status.Message)
	}
}

func TestNetworkLoadBalancerControllerActivatesAndStoresAppliedHash(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupNetworkLoadBalancerTestHooks(t, func(script string) error { return nil })
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-active", "web-servers", "192.168.1.181")

	ctrl.reconcileNetworkLoadBalancerDesiredState(created)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_ACTIVE {
		t.Fatalf("network load balancer status = %v, want %d(ACTIVE)", after.Status, db.NETWORK_LOAD_BALANCER_ACTIVE)
	}
	if got := networkLoadBalancerAppliedConfigHash(after); got == "" {
		t.Fatalf("applied config hash should not be empty")
	}
}

func TestNetworkLoadBalancerControllerActiveDriftTransitionsToConfiguring(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupNetworkLoadBalancerTestHooks(t, func(script string) error { return nil })
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-drift", "web-servers", "192.168.1.182")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_ACTIVE, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}
	if err := database.UpdateNetworkLoadBalancerById(api.NetworkLoadBalancerID(created), api.NetworkLoadBalancer{
		Metadata: api.Metadata{Labels: &map[string]interface{}{db.NetworkLoadBalancerLabelAppliedConfig: "stalehash"}},
	}); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerById() failed: %v", err)
	}

	activeItem, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile: %v", err)
	}

	ctrl.reconcileNetworkLoadBalancerDesiredState(activeItem)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed after reconcile: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_CONFIGURING {
		t.Fatalf("network load balancer status = %v, want %d(CONFIGURING)", after.Status, db.NETWORK_LOAD_BALANCER_CONFIGURING)
	}
	if got := networkLoadBalancerAppliedConfigHash(after); got != "stalehash" {
		t.Fatalf("applied config hash should remain stale until successful apply, got %q", got)
	}
}

func TestNetworkLoadBalancerControllerConfiguringAppliesAndActivates(t *testing.T) {
	database := newGatewayTestDatabase(t)
	called := false
	setupNetworkLoadBalancerTestHooks(t, func(script string) error {
		called = true
		if strings.TrimSpace(script) == "" {
			t.Fatalf("iptables restore script should not be empty")
		}
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-configuring", "web-servers", "192.168.1.183")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_CONFIGURING, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}
	if err := database.UpdateNetworkLoadBalancerById(api.NetworkLoadBalancerID(created), api.NetworkLoadBalancer{
		Metadata: api.Metadata{Labels: &map[string]interface{}{db.NetworkLoadBalancerLabelApplyRetries: 2}},
	}); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerById() failed: %v", err)
	}

	configuringItem, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile: %v", err)
	}

	ctrl.reconcileNetworkLoadBalancerDesiredState(configuringItem)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed after reconcile: %v", err)
	}
	if !called {
		t.Fatalf("expected iptables apply runner to be called")
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_ACTIVE {
		t.Fatalf("network load balancer status = %v, want %d(ACTIVE)", after.Status, db.NETWORK_LOAD_BALANCER_ACTIVE)
	}
	if got := networkLoadBalancerAppliedConfigHash(after); got == "" {
		t.Fatalf("applied config hash should not be empty")
	}
	if got := networkLoadBalancerApplyRetries(after); got != 0 {
		t.Fatalf("apply retries should be reset to 0 after successful apply, got %d", got)
	}
}

func TestNetworkLoadBalancerControllerApplyFailureRetriesThenFailed(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupNetworkLoadBalancerTestHooks(t, func(script string) error {
		return fmt.Errorf("mock apply failure")
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-retry-fail", "web-servers", "192.168.1.184")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_CONFIGURING, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}

	for i := 1; i <= networkLoadBalancerApplyRetryMax; i++ {
		item, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
		if err != nil {
			t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile: %v", err)
		}

		if i > 1 {
			currentRetries := networkLoadBalancerApplyRetries(item)
			if err := database.UpdateNetworkLoadBalancerById(api.NetworkLoadBalancerID(created), api.NetworkLoadBalancer{
				Metadata: api.Metadata{Labels: &map[string]interface{}{
					db.NetworkLoadBalancerLabelApplyRetries:  currentRetries,
					db.NetworkLoadBalancerLabelApplyFailedAt: time.Now().UTC().Add(-networkLoadBalancerApplyRetryBackoff - time.Second).Format(time.RFC3339Nano),
				}},
			}); err != nil {
				t.Fatalf("UpdateNetworkLoadBalancerById() failed while aging backoff timestamp: %v", err)
			}
			item, err = database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
			if err != nil {
				t.Fatalf("GetNetworkLoadBalancerById() failed after aging backoff timestamp: %v", err)
			}
		}
		ctrl.reconcileNetworkLoadBalancerDesiredState(item)
	}

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed after reconcile: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_FAILED {
		t.Fatalf("network load balancer status = %v, want %d(FAILED)", after.Status, db.NETWORK_LOAD_BALANCER_FAILED)
	}
	if got := networkLoadBalancerApplyRetries(after); got != networkLoadBalancerApplyRetryMax {
		t.Fatalf("apply retries = %d, want %d", got, networkLoadBalancerApplyRetryMax)
	}
}

func TestNetworkLoadBalancerControllerApplyFailureMarksConfiguringBeforeThreshold(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupNetworkLoadBalancerTestHooks(t, func(script string) error {
		return fmt.Errorf("mock apply failure")
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-retry-configuring", "web-servers", "192.168.1.185")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_CONFIGURING, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}

	item, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile: %v", err)
	}
	ctrl.reconcileNetworkLoadBalancerDesiredState(item)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed after reconcile: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_CONFIGURING {
		t.Fatalf("network load balancer status = %v, want %d(CONFIGURING)", after.Status, db.NETWORK_LOAD_BALANCER_CONFIGURING)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "retrying") {
		t.Fatalf("status message = %v, want retrying message", after.Status.Message)
	}
	if got := networkLoadBalancerApplyRetries(after); got != 1 {
		t.Fatalf("apply retries = %d, want 1", got)
	}
}

func TestNetworkLoadBalancerControllerApplyFailureBackoffDefersRetry(t *testing.T) {
	database := newGatewayTestDatabase(t)
	called := 0
	setupNetworkLoadBalancerTestHooks(t, func(script string) error {
		called++
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	mustCreateLoadBalancerBackendServer(t, database, "web-a", "web-servers", "172.16.10.11")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-retry-backoff", "web-servers", "192.168.1.186")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_CONFIGURING, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}
	if err := database.UpdateNetworkLoadBalancerById(api.NetworkLoadBalancerID(created), api.NetworkLoadBalancer{
		Metadata: api.Metadata{Labels: &map[string]interface{}{
			db.NetworkLoadBalancerLabelApplyRetries:  1,
			db.NetworkLoadBalancerLabelApplyFailedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}},
	}); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerById() failed: %v", err)
	}

	item, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile: %v", err)
	}
	ctrl.reconcileNetworkLoadBalancerDesiredState(item)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed after reconcile: %v", err)
	}
	if called != 0 {
		t.Fatalf("apply runner should be deferred during backoff, called=%d", called)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_CONFIGURING {
		t.Fatalf("network load balancer status = %v, want %d(CONFIGURING)", after.Status, db.NETWORK_LOAD_BALANCER_CONFIGURING)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "deferred") {
		t.Fatalf("status message = %v, want deferred retry message", after.Status.Message)
	}
	if got := networkLoadBalancerApplyRetries(after); got != 1 {
		t.Fatalf("apply retries should stay unchanged during backoff, got %d", got)
	}
}

func TestNetworkLoadBalancerControllerDeletingCleansAndDeletes(t *testing.T) {
	database := newGatewayTestDatabase(t)
	cleanupCalled := false
	setupNetworkLoadBalancerCleanupTestHooks(t, func(spec api.NetworkLoadBalancerSpec, chainPrefix string) error {
		cleanupCalled = true
		if strings.TrimSpace(spec.BindPublicIpAddress) == "" {
			t.Fatalf("cleanup spec bindPublicIpAddress should not be empty")
		}
		if !strings.HasPrefix(strings.TrimSpace(chainPrefix), "NLB_") {
			t.Fatalf("cleanup chainPrefix should use NLB_<id> format, got %q", chainPrefix)
		}
		return nil
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-delete", "web-servers", "192.168.1.187")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_DELETING, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}

	deletingItem, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile deleting: %v", err)
	}
	ctrl.reconcileNetworkLoadBalancerDeleting(deletingItem)

	if !cleanupCalled {
		t.Fatalf("expected cleanup runner to be called")
	}
	_, err = database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != db.ErrNotFound {
		t.Fatalf("GetNetworkLoadBalancerById() error = %v, want ErrNotFound", err)
	}
}

func TestNetworkLoadBalancerControllerDeletingCleanupFailureKeepsObject(t *testing.T) {
	database := newGatewayTestDatabase(t)
	setupNetworkLoadBalancerCleanupTestHooks(t, func(spec api.NetworkLoadBalancerSpec, chainPrefix string) error {
		return fmt.Errorf("mock cleanup failure")
	})
	ctrl := &controller{
		db:            database,
		marmot:        &marmotd.Marmot{NodeName: "hvc", Db: database},
		deletionDelay: 15 * time.Second,
	}

	_ = mustCreateVirtualNetwork(t, database, "web-servers")
	created := mustCreateNetworkLoadBalancer(t, database, "nlb-delete-retry", "web-servers", "192.168.1.188")

	if err := database.UpdateNetworkLoadBalancerStatusWithMessage(api.NetworkLoadBalancerID(created), db.NETWORK_LOAD_BALANCER_DELETING, ""); err != nil {
		t.Fatalf("UpdateNetworkLoadBalancerStatusWithMessage() failed: %v", err)
	}

	deletingItem, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() failed before reconcile deleting: %v", err)
	}
	ctrl.reconcileNetworkLoadBalancerDeleting(deletingItem)

	after, err := database.GetNetworkLoadBalancerById(api.NetworkLoadBalancerID(created))
	if err != nil {
		t.Fatalf("GetNetworkLoadBalancerById() should keep object on cleanup failure: %v", err)
	}
	if after.Status == nil || after.Status.StatusCode != db.NETWORK_LOAD_BALANCER_DELETING {
		t.Fatalf("network load balancer status = %v, want %d(DELETING)", after.Status, db.NETWORK_LOAD_BALANCER_DELETING)
	}
	if after.Status.Message == nil || !strings.Contains(*after.Status.Message, "cleanup failed, retrying") {
		t.Fatalf("status message = %v, want cleanup retry message", after.Status.Message)
	}
}

func setupNetworkLoadBalancerTestHooks(t *testing.T, runner func(script string) error) {
	t.Helper()
	oldRunner := runNetworkLoadBalancerApply
	runNetworkLoadBalancerApply = runner
	t.Cleanup(func() {
		runNetworkLoadBalancerApply = oldRunner
	})
}

func setupNetworkLoadBalancerCleanupTestHooks(t *testing.T, runner func(spec api.NetworkLoadBalancerSpec, chainPrefix string) error) {
	t.Helper()
	oldRunner := runNetworkLoadBalancerCleanup
	runNetworkLoadBalancerCleanup = runner
	t.Cleanup(func() {
		runNetworkLoadBalancerCleanup = oldRunner
	})
}

func TestNetworkLoadBalancerControllerSettingsFromEnv(t *testing.T) {
	oldRetryMax := networkLoadBalancerApplyRetryMax
	oldBackoff := networkLoadBalancerApplyRetryBackoff
	t.Cleanup(func() {
		networkLoadBalancerApplyRetryMax = oldRetryMax
		networkLoadBalancerApplyRetryBackoff = oldBackoff
	})

	t.Setenv("MARMOT_NLB_APPLY_RETRY_MAX", "7")
	t.Setenv("MARMOT_NLB_APPLY_RETRY_BACKOFF_SECONDS", "21")

	networkLoadBalancerControllerSettingsFromEnv()

	if networkLoadBalancerApplyRetryMax != 7 {
		t.Fatalf("networkLoadBalancerApplyRetryMax = %d, want 7", networkLoadBalancerApplyRetryMax)
	}
	if networkLoadBalancerApplyRetryBackoff != 21*time.Second {
		t.Fatalf("networkLoadBalancerApplyRetryBackoff = %v, want %v", networkLoadBalancerApplyRetryBackoff, 21*time.Second)
	}
}

func mustCreateNetworkLoadBalancer(t *testing.T, database *db.Database, name, internalNetwork, publicIP string) api.NetworkLoadBalancer {
	t.Helper()
	item, err := database.CreateNetworkLoadBalancer(api.NetworkLoadBalancer{
		ApiVersion: "v1",
		Kind:       "NetworkLoadBalancer",
		Metadata: api.Metadata{
			Name:     name,
			NodeName: util.StringPtr("hvc"),
		},
		Spec: api.NetworkLoadBalancerSpec{
			BindPublicIpAddress:    publicIP,
			InternalVirtualNetwork: internalNetwork,
			RemoteCIDR:             "0.0.0.0/0",
			Listeners: []api.NetworkLoadBalancerListener{{
				Name:        "tcp-80",
				Protocol:    "tcp",
				VipPort:     80,
				BackendPort: 8080,
				BackendSelector: api.NetworkLoadBalancerLabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateNetworkLoadBalancer() failed: %v", err)
	}
	if api.NetworkLoadBalancerID(item) == "" {
		t.Fatalf("CreateNetworkLoadBalancer() returned empty id")
	}
	return item
}
