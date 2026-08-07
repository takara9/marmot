package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

func newTestKubernetesEngine(t *testing.T, database *db.Database, name string) api.KubernetesEngine {
	t.Helper()
	ke, err := database.CreateKubernetesEngine(api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata:   api.Metadata{Name: name},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesEngine(%q) failed: %v", name, err)
	}
	return ke
}

// Pending: 専用ネットワークが1つだけ作成され、node割当・ラベルが正しく設定され、PROVISIONINGへ遷移することを確認する。
func TestReconcileKubernetesEnginePendingCreatesAssignedNetworkAndTransitions(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &kubernetesEngineController{db: database, node: "node-a"}

	ke := newTestKubernetesEngine(t, database, "demo")
	id := api.KubernetesEngineID(ke)
	networkName := kubernetesEngineNetworkName(ke)

	ctrl.reconcileKubernetesEnginePending(ke)

	network, err := database.GetVirtualNetworkByName(networkName)
	if err != nil {
		t.Fatalf("GetVirtualNetworkByName(%q) failed: %v", networkName, err)
	}
	if network.Metadata.NodeName == nil || *network.Metadata.NodeName != "node-a" {
		t.Fatalf("network NodeName = %v, want node-a", network.Metadata.NodeName)
	}
	if network.Metadata.Labels == nil {
		t.Fatalf("network labels are nil")
	}
	labels := *network.Metadata.Labels
	if owner, _ := labels[db.KubernetesEngineNetworkLabelOwner].(string); owner != id {
		t.Fatalf("owner label = %v, want %v", owner, id)
	}
	if managedBy, _ := labels[db.KubernetesEngineNetworkLabelManagedBy].(string); managedBy != db.KubernetesEngineNetworkLabelManagedByValue {
		t.Fatalf("managedBy label = %v, want %v", managedBy, db.KubernetesEngineNetworkLabelManagedByValue)
	}
	if syncRole := db.GetNetworkSyncRole(labels); syncRole != "head" {
		t.Fatalf("syncRole label = %v, want head", syncRole)
	}

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_PROVISIONING {
		t.Fatalf("status = %+v, want PROVISIONING", updated.Status)
	}

	// 再実行しても同じネットワークが1つのままであること(冪等性)を確認する。
	ctrl.reconcileKubernetesEnginePending(ke)
	networks, err := database.GetVirtualNetworks()
	if err != nil {
		t.Fatalf("GetVirtualNetworks() failed: %v", err)
	}
	count := 0
	for _, n := range networks {
		if n.Metadata.Name == networkName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("network count for %q = %d, want 1", networkName, count)
	}
}

// Pending: 同名ネットワークが別所有者に使われている場合はネットワークを作成せず、PENDINGのまま留まる(次tickで再試行)ことを確認する。
func TestReconcileKubernetesEnginePendingRetriesOnNameCollision(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &kubernetesEngineController{db: database, node: "node-a"}

	ke := newTestKubernetesEngine(t, database, "collide")
	id := api.KubernetesEngineID(ke)
	networkName := kubernetesEngineNetworkName(ke)

	// 所有者ラベルの無い同名ネットワークを事前に作成しておく(名前の衝突を再現)。
	if _, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: networkName},
	}); err != nil {
		t.Fatalf("CreateVirtualNetwork(%q) failed: %v", networkName, err)
	}

	ctrl.reconcileKubernetesEnginePending(ke)

	updated, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}
	if updated.Status == nil || updated.Status.StatusCode != db.KUBERNETES_ENGINE_PENDING {
		t.Fatalf("status = %+v, want to remain PENDING after name collision", updated.Status)
	}
}

// Deleting: 削除タイムスタンプは1度だけ設定され(再実行してもリセットされない)、ネットワークが
// 存在する間はクラスタ本体は削除されず、ネットワークがErrNotFoundになって初めて実削除されることを確認する。
func TestReconcileKubernetesEngineDeletingWaitsForNetworkRemoval(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &kubernetesEngineController{db: database, node: "node-a"}

	ke := newTestKubernetesEngine(t, database, "teardown")
	id := api.KubernetesEngineID(ke)
	networkName := kubernetesEngineNetworkName(ke)

	ctrl.reconcileKubernetesEnginePending(ke)
	ke, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	ctrl.reconcileKubernetesEngineDeleting(ke)

	network, err := database.GetVirtualNetworkByName(networkName)
	if err != nil {
		t.Fatalf("GetVirtualNetworkByName(%q) failed: %v", networkName, err)
	}
	if network.Status == nil || network.Status.DeletionTimeStamp == nil {
		t.Fatalf("network DeletionTimeStamp not set after first Deleting reconcile")
	}
	firstTimestamp := *network.Status.DeletionTimeStamp

	if _, err := database.GetKubernetesEngineById(id); err != nil {
		t.Fatalf("kubernetes engine was deleted while network still exists: %v", err)
	}

	// 2回目のreconcileでタイムスタンプがリセットされない(冪等)ことを確認する。
	ctrl.reconcileKubernetesEngineDeleting(ke)
	network, err = database.GetVirtualNetworkByName(networkName)
	if err != nil {
		t.Fatalf("GetVirtualNetworkByName(%q) failed after 2nd reconcile: %v", networkName, err)
	}
	if network.Status == nil || network.Status.DeletionTimeStamp == nil || !network.Status.DeletionTimeStamp.Equal(firstTimestamp) {
		t.Fatalf("DeletionTimeStamp was reset: got %v, want %v", network.Status.DeletionTimeStamp, firstTimestamp)
	}

	// ネットワークコントローラーによる実削除完了を模擬する。
	if err := database.DeleteVirtualNetworkById(api.VirtualNetworkID(network)); err != nil {
		t.Fatalf("DeleteVirtualNetworkById() failed: %v", err)
	}

	ctrl.reconcileKubernetesEngineDeleting(ke)

	if _, err := database.GetKubernetesEngineById(id); err != db.ErrNotFound {
		t.Fatalf("GetKubernetesEngineById() after network removal = %v, want ErrNotFound", err)
	}
}

// Deleting: フォロワーエントリはヘッドの所有者ラベルを引き継ぐため、ヘッド削除後もフォロワーが
// 残っている間はクラスタ本体が削除されず、フォロワーも削除されて初めて実削除されることを確認する。
func TestReconcileKubernetesEngineDeletingWaitsForFollowerNetworkRemoval(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &kubernetesEngineController{db: database, node: "node-a"}

	ke := newTestKubernetesEngine(t, database, "teardown-follower")
	id := api.KubernetesEngineID(ke)
	networkName := kubernetesEngineNetworkName(ke)

	ctrl.reconcileKubernetesEnginePending(ke)
	ke, err := database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	head, err := database.GetVirtualNetworkByName(networkName)
	if err != nil {
		t.Fatalf("GetVirtualNetworkByName(%q) failed: %v", networkName, err)
	}
	headID := api.VirtualNetworkID(head)

	followerID, err := database.MakeFollowerVirtualNetworkEntry(head, "node-b", headID)
	if err != nil {
		t.Fatalf("MakeFollowerVirtualNetworkEntry() failed: %v", err)
	}

	follower, err := database.GetVirtualNetworkById(followerID)
	if err != nil {
		t.Fatalf("GetVirtualNetworkById(%q) failed: %v", followerID, err)
	}
	if follower.Metadata.Labels == nil {
		t.Fatalf("follower labels are nil")
	}
	followerLabels := *follower.Metadata.Labels
	if owner, _ := followerLabels[db.KubernetesEngineNetworkLabelOwner].(string); owner != id {
		t.Fatalf("follower owner label = %v, want %v", owner, id)
	}
	if managedBy, _ := followerLabels[db.KubernetesEngineNetworkLabelManagedBy].(string); managedBy != db.KubernetesEngineNetworkLabelManagedByValue {
		t.Fatalf("follower managedBy label = %v, want %v", managedBy, db.KubernetesEngineNetworkLabelManagedByValue)
	}

	// ヘッドネットワークを先に削除し、フォロワーのみが残っている状態を再現する。
	if err := database.DeleteVirtualNetworkById(headID); err != nil {
		t.Fatalf("DeleteVirtualNetworkById(head) failed: %v", err)
	}

	ctrl.reconcileKubernetesEngineDeleting(ke)

	if _, err := database.GetKubernetesEngineById(id); err != nil {
		t.Fatalf("kubernetes engine was deleted while follower network still exists: %v", err)
	}

	follower, err = database.GetVirtualNetworkById(followerID)
	if err != nil {
		t.Fatalf("GetVirtualNetworkById(%q) failed: %v", followerID, err)
	}
	if follower.Status == nil || follower.Status.DeletionTimeStamp == nil {
		t.Fatalf("follower DeletionTimeStamp not set after Deleting reconcile")
	}

	// フォロワーの実削除完了を模擬する。
	if err := database.DeleteVirtualNetworkById(followerID); err != nil {
		t.Fatalf("DeleteVirtualNetworkById(follower) failed: %v", err)
	}

	ctrl.reconcileKubernetesEngineDeleting(ke)

	if _, err := database.GetKubernetesEngineById(id); err != db.ErrNotFound {
		t.Fatalf("GetKubernetesEngineById() after follower removal = %v, want ErrNotFound", err)
	}
}
