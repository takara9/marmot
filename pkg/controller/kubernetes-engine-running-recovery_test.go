package controller

import (
	"errors"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

// reconcileKubernetesEngineRunning は、marmotdホストの再起動等でコントロールプレーン専用の
// ネットワークネームスペース(/run/netns配下、tmpfsのため再起動で消える)が失われている場合に、
// コントロールプレーンの再プロビジョニングで自動復旧を試みることを確認する。
func TestReconcileKubernetesEngineRunningRecoversMissingControlPlaneNamespace(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "ns-recover"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	origNamespaceExists := controlPlaneNamespaceExists
	origProvisionControlPlane := provisionKubernetesEngineControlPlane
	t.Cleanup(func() {
		controlPlaneNamespaceExists = origNamespaceExists
		provisionKubernetesEngineControlPlane = origProvisionControlPlane
	})

	controlPlaneNamespaceExists = func(string) bool { return false }
	var recoverCalled bool
	provisionKubernetesEngineControlPlane = func(_ *db.Database, _ *marmotd.MKEConfig, _ string, _ api.KubernetesEngine) error {
		recoverCalled = true
		return nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	if !recoverCalled {
		t.Fatalf("provisionKubernetesEngineControlPlane was not called to recover the missing namespace")
	}
	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want to remain RUNNING after recovery", updated.Status)
	}
}

// ネームスペースが健全な場合、不要な再プロビジョニングを避けるため
// provisionKubernetesEngineControlPlane は呼び出されないことを確認する。
func TestReconcileKubernetesEngineRunningSkipsRecoveryWhenNamespaceHealthy(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "ns-healthy"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	origNamespaceExists := controlPlaneNamespaceExists
	origProvisionControlPlane := provisionKubernetesEngineControlPlane
	t.Cleanup(func() {
		controlPlaneNamespaceExists = origNamespaceExists
		provisionKubernetesEngineControlPlane = origProvisionControlPlane
	})

	controlPlaneNamespaceExists = func(string) bool { return true }
	provisionKubernetesEngineControlPlane = func(_ *db.Database, _ *marmotd.MKEConfig, _ string, _ api.KubernetesEngine) error {
		t.Fatalf("provisionKubernetesEngineControlPlane should not be called when namespace is healthy")
		return nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)
}

// 復旧に失敗した場合はエラーメッセージを記録しつつ、StatusCodeはRUNNINGのまま次回tickで
// 再試行できるようにすることを確認する。
func TestReconcileKubernetesEngineRunningRecordsMessageWhenRecoveryFails(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "ns-recover-fail"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	origNamespaceExists := controlPlaneNamespaceExists
	origProvisionControlPlane := provisionKubernetesEngineControlPlane
	t.Cleanup(func() {
		controlPlaneNamespaceExists = origNamespaceExists
		provisionKubernetesEngineControlPlane = origProvisionControlPlane
	})

	controlPlaneNamespaceExists = func(string) bool { return false }
	provisionKubernetesEngineControlPlane = func(_ *db.Database, _ *marmotd.MKEConfig, _ string, _ api.KubernetesEngine) error {
		return errors.New("simulated recovery failure")
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want to remain RUNNING so the next tick retries recovery", updated.Status)
	}
	if updated.Status.Message == nil || *updated.Status.Message == "" {
		t.Fatalf("status message was not recorded for the recovery failure")
	}
}

// ノードVMの再起動で`ip route replace`により設定した経路が失われても、RUNNING状態の間は
// 毎tick reconcileKubernetesEngineRunningRoutes が呼び出されて再設定が試みられることを確認する。
func TestReconcileKubernetesEngineRunningReconciliatesNodeRoutes(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "routes-recover"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	origNamespaceExists := controlPlaneNamespaceExists
	origRunningRoutes := reconcileKubernetesEngineRunningRoutes
	t.Cleanup(func() {
		controlPlaneNamespaceExists = origNamespaceExists
		reconcileKubernetesEngineRunningRoutes = origRunningRoutes
	})

	controlPlaneNamespaceExists = func(string) bool { return true }
	var reconciledID string
	reconcileKubernetesEngineRunningRoutes = func(_ *db.Database, targetKe api.KubernetesEngine) error {
		reconciledID = api.KubernetesEngineID(targetKe)
		return nil
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	if reconciledID != id {
		t.Fatalf("reconcileKubernetesEngineRunningRoutes was not called for id %q, got %q", id, reconciledID)
	}
}

// 経路の再設定に失敗した場合はメッセージを記録しつつ、RUNNINGのまま次回tickで
// 再試行できるようにすることを確認する。
func TestReconcileKubernetesEngineRunningRecordsMessageWhenRouteReconciliationFails(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "routes-recover-fail"},
		Spec:       api.KubernetesEngineSpec{Version: "1.36", Nodes: 1},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine() failed: %v", err)
	}
	id := api.KubernetesEngineID(ke)
	if err := database.UpdateKubernetesEngineStatusWithMessage(id, db.KUBERNETES_ENGINE_RUNNING, ""); err != nil {
		t.Fatalf("failed to set RUNNING: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	origNamespaceExists := controlPlaneNamespaceExists
	origRunningRoutes := reconcileKubernetesEngineRunningRoutes
	t.Cleanup(func() {
		controlPlaneNamespaceExists = origNamespaceExists
		reconcileKubernetesEngineRunningRoutes = origRunningRoutes
	})

	controlPlaneNamespaceExists = func(string) bool { return true }
	reconcileKubernetesEngineRunningRoutes = func(_ *db.Database, _ api.KubernetesEngine) error {
		return errors.New("simulated route reconciliation failure")
	}

	ctrl := &kubernetesEngineController{db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineRunning(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_RUNNING {
		t.Fatalf("status = %+v, want to remain RUNNING so the next tick retries route reconciliation", updated.Status)
	}
	if updated.Status.Message == nil || *updated.Status.Message == "" {
		t.Fatalf("status message was not recorded for the route reconciliation failure")
	}
}
