package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestIsKubernetesEngineOwnerHost(t *testing.T) {
	tests := []struct {
		name     string
		node     string
		nodeName *string
		want     bool
	}{
		{name: "unset NodeName treated as owner (backward compatibility)", node: "hv3", nodeName: nil, want: true},
		{name: "matching NodeName is owner", node: "hv3", nodeName: util.StringPtr("hv3"), want: true},
		{name: "different NodeName is not owner", node: "hv3", nodeName: util.StringPtr("hv4"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := &kubernetesEngineController{node: tt.node}
			ke := api.KubernetesEngine{Metadata: api.Metadata{NodeName: tt.nodeName}}
			if got := ctrl.isKubernetesEngineOwnerHost(ke); got != tt.want {
				t.Fatalf("isKubernetesEngineOwnerHost() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReconcileKubernetesEngineProvisioningSkipsOnNonOwnerHost は、marmotクラスタ内の
// 別ホストが所有するKubernetesEngineに対しては、このホストがコントロールプレーンを
// 重複して構築しないことを確認する(スプリットブレイン防止)。
func TestReconcileKubernetesEngineProvisioningSkipsOnNonOwnerHost(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: "demo-owner-guard", NodeName: util.StringPtr("hv4")},
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
	t.Cleanup(func() { provisionKubernetesEngineControlPlane = oldControlPlane })
	called := false
	provisionKubernetesEngineControlPlane = func(*db.Database, *marmotd.MKEConfig, string, api.KubernetesEngine) error {
		called = true
		return nil
	}

	// このホストは"hv3"だが、KEの所有ホストは"hv4"なので何もしてはいけない。
	ctrl := &kubernetesEngineController{node: "hv3", db: database, mkeConf: &marmotd.MKEConfig{}, etcdURL: "http://127.0.0.1:2379"}
	ctrl.reconcileKubernetesEngineProvisioning(ke)

	if called {
		t.Fatalf("provisionKubernetesEngineControlPlane was called on a non-owner host")
	}
	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_PROVISIONING {
		t.Fatalf("status = %+v, want unchanged PROVISIONING", updated.Status)
	}
}
