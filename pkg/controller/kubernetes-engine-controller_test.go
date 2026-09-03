package controller

import (
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
