package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

func TestIsKubernetesEngineInGracePeriod(t *testing.T) {
	withinDelay := time.Now().Add(-5 * time.Second)
	expiredDelay := time.Now().Add(-(KUBERNETES_ENGINE_DELETION_DELAY + time.Second))

	tests := []struct {
		name string
		ke   api.KubernetesEngine
		want bool
	}{
		{
			name: "no status: not in grace period",
			ke:   api.KubernetesEngine{},
			want: false,
		},
		{
			name: "nil DeletionTimeStamp: not in grace period",
			ke:   api.KubernetesEngine{Status: &api.Status{StatusCode: db.KUBERNETES_ENGINE_RUNNING}},
			want: false,
		},
		{
			name: "timestamp within delay: record is in grace period and must be preserved",
			ke: api.KubernetesEngine{
				Status: &api.Status{
					StatusCode:        db.KUBERNETES_ENGINE_DELETING,
					DeletionTimeStamp: &withinDelay,
				},
			},
			want: true,
		},
		{
			name: "timestamp past delay: grace period expired and record may be hard-deleted",
			ke: api.KubernetesEngine{
				Status: &api.Status{
					StatusCode:        db.KUBERNETES_ENGINE_DELETING,
					DeletionTimeStamp: &expiredDelay,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKubernetesEngineInGracePeriod(tt.ke)
			if got != tt.want {
				t.Fatalf("isKubernetesEngineInGracePeriod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileKubernetesEngineProvisioningTransitionsWhenNodesReady(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-ready"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_PROVISIONING, ""); err != nil {
		t.Fatalf("failed to set PROVISIONING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldControlPlane := provisionKubernetesEngineControlPlane
	oldNodes := provisionKubernetesEngineNodes
	t.Cleanup(func() {
		provisionKubernetesEngineControlPlane = oldControlPlane
		provisionKubernetesEngineNodes = oldNodes
	})
	provisionKubernetesEngineControlPlane = func(database *db.Database, _ *marmotd.MKEConfig, _ string, current api.KubernetesEngine) error {
		return database.UpdateKubernetesEngineControlPlaneStatus(api.KubernetesEngineID(current), "172.16.1.2", "172.16.1.3", 26443, "v1.36.2")
	}
	provisionKubernetesEngineNodes = func(_ *db.Database, _ *marmotd.MKEConfig, current api.KubernetesEngine) (bool, error) {
		if current.Status == nil || current.Status.ControlPlaneIpAddress == nil {
			t.Fatalf("node provisioning received stale control plane status: %+v", current.Status)
		}
		return true, nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineProvisioning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want RUNNING", updated.Status)
	}
}

func TestReconcileKubernetesEngineRunningScalesOutWhenNodesIncrease(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-scale-out"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	if err := database.UpdateKubernetesEngineSpec(id, api.KubernetesEngineSpec{Version: "1.36", Nodes: 2}); err != nil {
		t.Fatalf("UpdateKubernetesEngineSpec() failed: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldNodes := provisionKubernetesEngineNodes
	t.Cleanup(func() { provisionKubernetesEngineNodes = oldNodes })
	provisionKubernetesEngineNodes = func(_ *db.Database, _ *marmotd.MKEConfig, _ api.KubernetesEngine) (bool, error) {
		return true, nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want RUNNING after scale-out completes", updated.Status)
	}
}

func TestReconcileKubernetesEngineRunningStaysScalingOutUntilNodesReady(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-scale-out-pending"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	if err := database.UpdateKubernetesEngineSpec(id, api.KubernetesEngineSpec{Version: "1.36", Nodes: 2}); err != nil {
		t.Fatalf("UpdateKubernetesEngineSpec() failed: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldNodes := provisionKubernetesEngineNodes
	t.Cleanup(func() { provisionKubernetesEngineNodes = oldNodes })
	provisionKubernetesEngineNodes = func(_ *db.Database, _ *marmotd.MKEConfig, _ api.KubernetesEngine) (bool, error) {
		return false, nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_SCALING_OUT {
		t.Fatalf("status = %+v, want SCALING_OUT while nodes are not yet ready", updated.Status)
	}
}

func createKubernetesEngineTestNodeServer(t *testing.T, database *db.Database, ke api.KubernetesEngine, index int) api.Server {
	t.Helper()
	id := api.KubernetesEngineID(ke)
	labels := map[string]interface{}{
		kubernetesEngineNodeLabelOwner: id,
		kubernetesEngineNodeLabelRole:  kubernetesEngineNodeRoleValue,
	}
	server, err := database.MakeServerEntry(api.Server{
		Metadata: api.Metadata{Name: kubernetesEngineNodeName(ke, index), Labels: &labels},
	})
	if err != nil {
		t.Fatalf("MakeServerEntry() failed: %v", err)
	}
	return server
}

func TestReconcileKubernetesEngineRunningScalesInWhenNodesDecrease(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-scale-in"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 2},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	createKubernetesEngineTestNodeServer(t, database, ke, 0)
	secondNode := createKubernetesEngineTestNodeServer(t, database, ke, 1)

	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	if err := database.UpdateKubernetesEngineSpec(id, api.KubernetesEngineSpec{Version: "1.36", Nodes: 1}); err != nil {
		t.Fatalf("UpdateKubernetesEngineSpec() failed: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldDrain := drainAndDeleteKubernetesEngineNode
	t.Cleanup(func() { drainAndDeleteKubernetesEngineNode = oldDrain })
	var drainedNode string
	drainAndDeleteKubernetesEngineNode = func(_ api.KubernetesEngine, nodeName string) error {
		drainedNode = nodeName
		return nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	wantNodeName := kubernetesEngineNodeName(ke, 1)
	if drainedNode != wantNodeName {
		t.Fatalf("drainAndDeleteKubernetesEngineNode called with node %q, want %q (highest index first)", drainedNode, wantNodeName)
	}
	updatedServer, err := database.GetServerById(api.ServerID(secondNode))
	if err != nil {
		t.Fatalf("GetServerById() failed: %v", err)
	}
	if updatedServer.Status == nil || updatedServer.Status.DeletionTimeStamp == nil {
		t.Fatalf("server %q was not marked for deletion after scale-in", secondNode.Metadata.Name)
	}
	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_SCALING_IN {
		t.Fatalf("status = %+v, want SCALING_IN while the removed node's VM deletion is still pending", updated.Status)
	}
}

func TestReconcileKubernetesEngineRunningScaleInWaitsForServerRemoval(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-scale-in-wait"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	createKubernetesEngineTestNodeServer(t, database, ke, 0)
	secondNode := createKubernetesEngineTestNodeServer(t, database, ke, 1)
	if err := database.SetDeleteTimestamp(api.ServerID(secondNode)); err != nil {
		t.Fatalf("SetDeleteTimestamp() failed: %v", err)
	}

	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_IN, ""); err != nil {
		t.Fatalf("failed to set SCALING_IN: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldDrain := drainAndDeleteKubernetesEngineNode
	t.Cleanup(func() { drainAndDeleteKubernetesEngineNode = oldDrain })
	drainAndDeleteKubernetesEngineNode = func(_ api.KubernetesEngine, _ string) error {
		t.Fatalf("drainAndDeleteKubernetesEngineNode should not be called while the excess node is already pending deletion")
		return nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_SCALING_IN {
		t.Fatalf("status = %+v, want to remain SCALING_IN until the server is actually removed", updated.Status)
	}
}

func TestReconcileKubernetesEngineRunningScaleInCompletesAfterServerRemoval(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-scale-in-done"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	createKubernetesEngineTestNodeServer(t, database, ke, 0)

	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_SCALING_IN, ""); err != nil {
		t.Fatalf("failed to set SCALING_IN: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want RUNNING once the excess node's server record is gone", updated.Status)
	}
}

func TestReconcileKubernetesEngineRunningTransitionsToUpgradingWhenVersionChanges(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-upgrade-trigger"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	createKubernetesEngineTestNodeServer(t, database, ke, 0)
	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	if err := database.UpdateKubernetesEngineSpec(id, api.KubernetesEngineSpec{Version: "1.37", Nodes: 1}); err != nil {
		t.Fatalf("UpdateKubernetesEngineSpec() failed: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_UPGRADING {
		t.Fatalf("status = %+v, want UPGRADING when spec.version differs from the resolved version", updated.Status)
	}
}

func TestReconcileKubernetesEngineUpgradingUpgradesControlPlaneBeforeNodes(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-upgrade-cp-first"},
		Spec:       api.KubernetesEngineSpec{Version: "1.37", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	createKubernetesEngineTestNodeServer(t, database, ke, 0)
	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.36.2"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, ""); err != nil {
		t.Fatalf("failed to set UPGRADING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldControlPlane := upgradeKubernetesEngineControlPlane
	oldNode := upgradeKubernetesEngineNode
	t.Cleanup(func() {
		upgradeKubernetesEngineControlPlane = oldControlPlane
		upgradeKubernetesEngineNode = oldNode
	})
	controlPlaneCalled := false
	upgradeKubernetesEngineControlPlane = func(database *db.Database, _ *marmotd.MKEConfig, _ string, current api.KubernetesEngine) error {
		controlPlaneCalled = true
		return database.UpdateKubernetesEngineControlPlaneStatus(api.KubernetesEngineID(current), "172.16.1.2", "172.16.1.3", 26443, "v1.37.0")
	}
	nodeCalled := false
	upgradeKubernetesEngineNode = func(_ *db.Database, _ *marmotd.MKEConfig, _ api.KubernetesEngine, _ api.Server) error {
		nodeCalled = true
		return nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineUpgrading(id, ke)

	if !controlPlaneCalled {
		t.Fatalf("expected the control plane upgrade to be called first")
	}
	if nodeCalled {
		t.Fatalf("node upgrade must not be called before the control plane upgrade completes")
	}
	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_UPGRADING {
		t.Fatalf("status = %+v, want to remain UPGRADING right after the control plane upgrade (node upgrades happen on later ticks)", updated.Status)
	}
	if updated.Status.ResolvedKubernetesVersion == nil || *updated.Status.ResolvedKubernetesVersion != "v1.37.0" {
		t.Fatalf("resolved version = %v, want v1.37.0 after the control plane upgrade", updated.Status.ResolvedKubernetesVersion)
	}
}

func TestReconcileKubernetesEngineUpgradingUpgradesNodesInAscendingIndexOrder(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-upgrade-node-order"},
		Spec:       api.KubernetesEngineSpec{Version: "1.37", Nodes: 2},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	createKubernetesEngineTestNodeServer(t, database, ke, 0)
	createKubernetesEngineTestNodeServer(t, database, ke, 1)
	// コントロールプレーンは既にspec.versionへ更新済みとし、ノードのローリングのみを検証する。
	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, "172.16.1.2", "172.16.1.3", 26443, "v1.37.0"); err != nil {
		t.Fatalf("failed to set control plane status: %v", err)
	}
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_UPGRADING, ""); err != nil {
		t.Fatalf("failed to set UPGRADING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	oldNode := upgradeKubernetesEngineNode
	t.Cleanup(func() { upgradeKubernetesEngineNode = oldNode })
	var upgradedOrder []string
	upgradeKubernetesEngineNode = func(database *db.Database, _ *marmotd.MKEConfig, current api.KubernetesEngine, server api.Server) error {
		upgradedOrder = append(upgradedOrder, server.Metadata.Name)
		labels := map[string]interface{}{}
		if server.Metadata.Labels != nil {
			labels = *server.Metadata.Labels
		}
		labels[kubernetesEngineNodeLabelKubeletVersion] = strings.TrimSpace(*current.Status.ResolvedKubernetesVersion)
		return database.UpdateServer(server.Metadata.Id, api.Server{Metadata: api.Metadata{Labels: &labels}})
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}

	ctrl.reconcileKubernetesEngineUpgrading(id, ke) // 1台目(index0)をアップグレード
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	ctrl.reconcileKubernetesEngineUpgrading(id, ke) // 2台目(index1)をアップグレード
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	ctrl.reconcileKubernetesEngineUpgrading(id, ke) // 全ノード完了→RUNNINGへ戻る

	wantOrder := []string{kubernetesEngineNodeName(ke, 0), kubernetesEngineNodeName(ke, 1)}
	if len(upgradedOrder) != 2 || upgradedOrder[0] != wantOrder[0] || upgradedOrder[1] != wantOrder[1] {
		t.Fatalf("upgrade order = %v, want %v (ascending index order)", upgradedOrder, wantOrder)
	}

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want RUNNING once every node has been upgraded", updated.Status)
	}
}
