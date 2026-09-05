package controller

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
)

// removeKubernetesEngineCephRBDImages: PV一覧から抽出したRBD-backed PVCをすべて削除要求し、
// 個々の削除失敗はエラーとして集約されるが処理自体は継続する(ベストエフォート)ことを確認する。
// 削除要求に成功したPVCについては、PV消失をポーリングで確認する。
func TestRemoveKubernetesEngineCephRBDImages(t *testing.T) {
	oldList := listKubernetesEngineCephRBDVolumesFn
	oldDelete := deleteKubernetesEnginePersistentVolumeClaimFn
	oldPods := listKubernetesEnginePodsUsingPVCFn
	oldForceDelete := forceDeleteKubernetesEnginePodFn
	oldSleep := kubernetesEngineCephRBDDeleteWaitSleepFn
	t.Cleanup(func() {
		listKubernetesEngineCephRBDVolumesFn = oldList
		deleteKubernetesEnginePersistentVolumeClaimFn = oldDelete
		listKubernetesEnginePodsUsingPVCFn = oldPods
		forceDeleteKubernetesEnginePodFn = oldForceDelete
		kubernetesEngineCephRBDDeleteWaitSleepFn = oldSleep
	})
	kubernetesEngineCephRBDDeleteWaitSleepFn = func(time.Duration) {}
	listKubernetesEnginePodsUsingPVCFn = func(_ api.KubernetesEngine, _, _ string) ([]kubernetesEngineMountingPod, error) {
		return nil, nil
	}
	forceDeleteKubernetesEnginePodFn = func(_ api.KubernetesEngine, _, _ string) error {
		return nil
	}

	initial := []kubernetesEngineCephRBDVolumeRef{
		{PVName: "pv-1", ClaimNamespace: "default", ClaimName: "claim-1"},
		{PVName: "pv-2", ClaimNamespace: "default", ClaimName: "claim-2"},
	}
	listCalls := 0
	listKubernetesEngineCephRBDVolumesFn = func(_ api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
		listCalls++
		if listCalls == 1 {
			return initial, nil
		}
		// pv-1のPVCは削除に成功しCeph-CSIが消し終えたが、pv-2は削除要求自体が失敗しているため残る。
		return []kubernetesEngineCephRBDVolumeRef{initial[1]}, nil
	}
	var deletedClaims []string
	deleteKubernetesEnginePersistentVolumeClaimFn = func(_ api.KubernetesEngine, namespace, name string) error {
		key := namespace + "/" + name
		if key == "default/claim-2" {
			return errors.New("delete pvc failed")
		}
		deletedClaims = append(deletedClaims, key)
		return nil
	}

	err := removeKubernetesEngineCephRBDImages(api.KubernetesEngine{})
	if err == nil {
		t.Fatalf("expected an aggregated error for the failing PVC, got nil")
	}
	if !strings.Contains(err.Error(), "claim-2") {
		t.Fatalf("error = %v, want it to mention the failing PVC", err)
	}
	if len(deletedClaims) != 1 || deletedClaims[0] != "default/claim-1" {
		t.Fatalf("deletedClaims = %v, want [default/claim-1]", deletedClaims)
	}
}

// PVCをマウント中のPodが見つかった場合、PVC削除の前にそのPodが強制削除されることを確認する
// (pvc-protection finalizerでPVC削除がブロックされるのを避けるため)。
func TestRemoveKubernetesEngineCephRBDImagesForceDeletesMountingPods(t *testing.T) {
	oldList := listKubernetesEngineCephRBDVolumesFn
	oldDelete := deleteKubernetesEnginePersistentVolumeClaimFn
	oldPods := listKubernetesEnginePodsUsingPVCFn
	oldForceDelete := forceDeleteKubernetesEnginePodFn
	oldResolve := resolveKubernetesEngineTopLevelControllerFn
	oldDeleteController := deleteKubernetesEngineControllerFn
	oldSleep := kubernetesEngineCephRBDDeleteWaitSleepFn
	t.Cleanup(func() {
		listKubernetesEngineCephRBDVolumesFn = oldList
		deleteKubernetesEnginePersistentVolumeClaimFn = oldDelete
		listKubernetesEnginePodsUsingPVCFn = oldPods
		forceDeleteKubernetesEnginePodFn = oldForceDelete
		resolveKubernetesEngineTopLevelControllerFn = oldResolve
		deleteKubernetesEngineControllerFn = oldDeleteController
		kubernetesEngineCephRBDDeleteWaitSleepFn = oldSleep
	})
	kubernetesEngineCephRBDDeleteWaitSleepFn = func(time.Duration) {}
	resolveKubernetesEngineTopLevelControllerFn = func(_ api.KubernetesEngine, _ string, owner kubernetesEnginePodOwnerRef) (kubernetesEnginePodOwnerRef, error) {
		return owner, nil
	}
	deleteKubernetesEngineControllerFn = func(_ api.KubernetesEngine, _ string, _ kubernetesEnginePodOwnerRef) error {
		return nil
	}

	ref := kubernetesEngineCephRBDVolumeRef{PVName: "pv-1", ClaimNamespace: "default", ClaimName: "claim-1"}
	listCalls := 0
	listKubernetesEngineCephRBDVolumesFn = func(_ api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
		listCalls++
		if listCalls == 1 {
			return []kubernetesEngineCephRBDVolumeRef{ref}, nil
		}
		// ポーリング時点ではPodの強制削除・PVC削除が完了しCeph-CSIがPVを消し終えていることを模擬する。
		return nil, nil
	}
	listKubernetesEnginePodsUsingPVCFn = func(_ api.KubernetesEngine, namespace, claimName string) ([]kubernetesEngineMountingPod, error) {
		if namespace != "default" || claimName != "claim-1" {
			t.Fatalf("listKubernetesEnginePodsUsingPVCFn called with unexpected args: %s/%s", namespace, claimName)
		}
		return []kubernetesEngineMountingPod{{Namespace: "default", Name: "app-pod"}}, nil
	}
	var order []string
	forceDeleteKubernetesEnginePodFn = func(_ api.KubernetesEngine, namespace, name string) error {
		order = append(order, "pod:"+namespace+"/"+name)
		return nil
	}
	deleteKubernetesEnginePersistentVolumeClaimFn = func(_ api.KubernetesEngine, namespace, name string) error {
		order = append(order, "pvc:"+namespace+"/"+name)
		return nil
	}

	if err := removeKubernetesEngineCephRBDImages(api.KubernetesEngine{}); err != nil {
		t.Fatalf("removeKubernetesEngineCephRBDImages() failed: %v", err)
	}
	want := []string{"pod:default/app-pod", "pvc:default/claim-1"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("order = %v, want %v (pod force-delete must happen before PVC delete)", order, want)
	}
}

// Podの所有コントローラー(ReplicaSet経由でDeploymentに解決される)は、Pod強制削除より先に削除され、
// 同一コントローラーへの削除は1回だけ行われることを確認する(kube-system以外が対象)。
func TestRemoveKubernetesEngineCephRBDImagesDeletesOwningControllerBeforePod(t *testing.T) {
	oldList := listKubernetesEngineCephRBDVolumesFn
	oldDelete := deleteKubernetesEnginePersistentVolumeClaimFn
	oldPods := listKubernetesEnginePodsUsingPVCFn
	oldForceDelete := forceDeleteKubernetesEnginePodFn
	oldResolve := resolveKubernetesEngineTopLevelControllerFn
	oldDeleteController := deleteKubernetesEngineControllerFn
	oldSleep := kubernetesEngineCephRBDDeleteWaitSleepFn
	t.Cleanup(func() {
		listKubernetesEngineCephRBDVolumesFn = oldList
		deleteKubernetesEnginePersistentVolumeClaimFn = oldDelete
		listKubernetesEnginePodsUsingPVCFn = oldPods
		forceDeleteKubernetesEnginePodFn = oldForceDelete
		resolveKubernetesEngineTopLevelControllerFn = oldResolve
		deleteKubernetesEngineControllerFn = oldDeleteController
		kubernetesEngineCephRBDDeleteWaitSleepFn = oldSleep
	})
	kubernetesEngineCephRBDDeleteWaitSleepFn = func(time.Duration) {}

	ref := kubernetesEngineCephRBDVolumeRef{PVName: "pv-1", ClaimNamespace: "default", ClaimName: "claim-1"}
	listCalls := 0
	listKubernetesEngineCephRBDVolumesFn = func(_ api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
		listCalls++
		if listCalls == 1 {
			return []kubernetesEngineCephRBDVolumeRef{ref}, nil
		}
		return nil, nil
	}
	owner := &kubernetesEnginePodOwnerRef{Kind: "ReplicaSet", Name: "app-rs-abc"}
	listKubernetesEnginePodsUsingPVCFn = func(_ api.KubernetesEngine, namespace, claimName string) ([]kubernetesEngineMountingPod, error) {
		return []kubernetesEngineMountingPod{
			{Namespace: "default", Name: "app-pod-1", Owner: owner},
			{Namespace: "default", Name: "app-pod-2", Owner: owner},
		}, nil
	}
	resolveCalls := 0
	resolveKubernetesEngineTopLevelControllerFn = func(_ api.KubernetesEngine, namespace string, o kubernetesEnginePodOwnerRef) (kubernetesEnginePodOwnerRef, error) {
		resolveCalls++
		if namespace != "default" || o.Kind != "ReplicaSet" || o.Name != "app-rs-abc" {
			t.Fatalf("resolveKubernetesEngineTopLevelControllerFn called with unexpected args: %s %+v", namespace, o)
		}
		return kubernetesEnginePodOwnerRef{Kind: "Deployment", Name: "app"}, nil
	}
	var order []string
	deleteKubernetesEngineControllerFn = func(_ api.KubernetesEngine, namespace string, o kubernetesEnginePodOwnerRef) error {
		order = append(order, "controller:"+namespace+"/"+o.Kind+"/"+o.Name)
		return nil
	}
	forceDeleteKubernetesEnginePodFn = func(_ api.KubernetesEngine, namespace, name string) error {
		order = append(order, "pod:"+namespace+"/"+name)
		return nil
	}
	deleteKubernetesEnginePersistentVolumeClaimFn = func(_ api.KubernetesEngine, namespace, name string) error {
		order = append(order, "pvc:"+namespace+"/"+name)
		return nil
	}

	if err := removeKubernetesEngineCephRBDImages(api.KubernetesEngine{}); err != nil {
		t.Fatalf("removeKubernetesEngineCephRBDImages() failed: %v", err)
	}
	want := []string{
		"controller:default/Deployment/app",
		"pod:default/app-pod-1",
		"pod:default/app-pod-2",
		"pvc:default/claim-1",
	}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	if resolveCalls != 2 {
		t.Fatalf("resolveKubernetesEngineTopLevelControllerFn called %d times, want 2 (once per pod)", resolveCalls)
	}
}

// PVCがkube-systemネームスペースにある場合は、参照Podの一覧取得・強制削除を行わずPVC削除のみ行う。
func TestRemoveKubernetesEngineCephRBDImagesSkipsForceDeleteInKubeSystem(t *testing.T) {
	oldList := listKubernetesEngineCephRBDVolumesFn
	oldDelete := deleteKubernetesEnginePersistentVolumeClaimFn
	oldPods := listKubernetesEnginePodsUsingPVCFn
	oldForceDelete := forceDeleteKubernetesEnginePodFn
	oldSleep := kubernetesEngineCephRBDDeleteWaitSleepFn
	t.Cleanup(func() {
		listKubernetesEngineCephRBDVolumesFn = oldList
		deleteKubernetesEnginePersistentVolumeClaimFn = oldDelete
		listKubernetesEnginePodsUsingPVCFn = oldPods
		forceDeleteKubernetesEnginePodFn = oldForceDelete
		kubernetesEngineCephRBDDeleteWaitSleepFn = oldSleep
	})
	kubernetesEngineCephRBDDeleteWaitSleepFn = func(time.Duration) {}

	ref := kubernetesEngineCephRBDVolumeRef{PVName: "pv-1", ClaimNamespace: "kube-system", ClaimName: "claim-1"}
	listCalls := 0
	listKubernetesEngineCephRBDVolumesFn = func(_ api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
		listCalls++
		if listCalls == 1 {
			return []kubernetesEngineCephRBDVolumeRef{ref}, nil
		}
		return nil, nil
	}
	listKubernetesEnginePodsUsingPVCFn = func(_ api.KubernetesEngine, namespace, claimName string) ([]kubernetesEngineMountingPod, error) {
		t.Fatalf("pod lookup should not be attempted for PVCs in kube-system")
		return nil, nil
	}
	forceDeleteKubernetesEnginePodFn = func(_ api.KubernetesEngine, namespace, name string) error {
		t.Fatalf("pod force-delete should not be attempted for PVCs in kube-system")
		return nil
	}
	var deletedClaims []string
	deleteKubernetesEnginePersistentVolumeClaimFn = func(_ api.KubernetesEngine, namespace, name string) error {
		deletedClaims = append(deletedClaims, namespace+"/"+name)
		return nil
	}

	if err := removeKubernetesEngineCephRBDImages(api.KubernetesEngine{}); err != nil {
		t.Fatalf("removeKubernetesEngineCephRBDImages() failed: %v", err)
	}
	if len(deletedClaims) != 1 || deletedClaims[0] != "kube-system/claim-1" {
		t.Fatalf("deletedClaims = %v, want [kube-system/claim-1]", deletedClaims)
	}
}

// pv-1のPVCが削除された後、ポーリングでPV消失を確認できれば、そのPVについてはエラーを返さない。
func TestRemoveKubernetesEngineCephRBDImagesWaitsForPVRemoval(t *testing.T) {
	oldList := listKubernetesEngineCephRBDVolumesFn
	oldDelete := deleteKubernetesEnginePersistentVolumeClaimFn
	oldPods := listKubernetesEnginePodsUsingPVCFn
	oldForceDelete := forceDeleteKubernetesEnginePodFn
	oldSleep := kubernetesEngineCephRBDDeleteWaitSleepFn
	t.Cleanup(func() {
		listKubernetesEngineCephRBDVolumesFn = oldList
		deleteKubernetesEnginePersistentVolumeClaimFn = oldDelete
		listKubernetesEnginePodsUsingPVCFn = oldPods
		forceDeleteKubernetesEnginePodFn = oldForceDelete
		kubernetesEngineCephRBDDeleteWaitSleepFn = oldSleep
	})
	listKubernetesEnginePodsUsingPVCFn = func(_ api.KubernetesEngine, _, _ string) ([]kubernetesEngineMountingPod, error) {
		return nil, nil
	}
	forceDeleteKubernetesEnginePodFn = func(_ api.KubernetesEngine, _, _ string) error {
		return nil
	}
	var slept []time.Duration
	kubernetesEngineCephRBDDeleteWaitSleepFn = func(d time.Duration) { slept = append(slept, d) }

	ref := kubernetesEngineCephRBDVolumeRef{PVName: "pv-1", ClaimNamespace: "default", ClaimName: "claim-1"}
	listCalls := 0
	listKubernetesEngineCephRBDVolumesFn = func(_ api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
		listCalls++
		if listCalls == 1 {
			return []kubernetesEngineCephRBDVolumeRef{ref}, nil
		}
		// 2回目のポーリングでPVが消えている(=Ceph-CSIが削除を完了した)ことを模擬する。
		if listCalls >= 3 {
			return nil, nil
		}
		return []kubernetesEngineCephRBDVolumeRef{ref}, nil
	}
	deleteKubernetesEnginePersistentVolumeClaimFn = func(_ api.KubernetesEngine, _, _ string) error {
		return nil
	}

	if err := removeKubernetesEngineCephRBDImages(api.KubernetesEngine{}); err != nil {
		t.Fatalf("removeKubernetesEngineCephRBDImages() failed: %v", err)
	}
	if len(slept) != 2 {
		t.Fatalf("slept %d times, want 2 (stops polling once the PV disappears)", len(slept))
	}
}

// PV一覧が空の場合は、Pod強制削除もPVC削除もポーリングも行わない。
func TestRemoveKubernetesEngineCephRBDImagesNoVolumes(t *testing.T) {
	oldList := listKubernetesEngineCephRBDVolumesFn
	oldDelete := deleteKubernetesEnginePersistentVolumeClaimFn
	oldPods := listKubernetesEnginePodsUsingPVCFn
	t.Cleanup(func() {
		listKubernetesEngineCephRBDVolumesFn = oldList
		deleteKubernetesEnginePersistentVolumeClaimFn = oldDelete
		listKubernetesEnginePodsUsingPVCFn = oldPods
	})

	listKubernetesEngineCephRBDVolumesFn = func(_ api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
		return nil, nil
	}
	listKubernetesEnginePodsUsingPVCFn = func(_ api.KubernetesEngine, _, _ string) ([]kubernetesEngineMountingPod, error) {
		t.Fatalf("pod lookup should not be attempted when there are no volumes")
		return nil, nil
	}
	deleteKubernetesEnginePersistentVolumeClaimFn = func(_ api.KubernetesEngine, _, _ string) error {
		t.Fatalf("PVC delete should not be attempted when there are no volumes")
		return nil
	}

	if err := removeKubernetesEngineCephRBDImages(api.KubernetesEngine{}); err != nil {
		t.Fatalf("removeKubernetesEngineCephRBDImages() failed: %v", err)
	}
}

// parseKubernetesEngineCephRBDVolumes: RBD以外(CephFS等)のPVは除外され、
// claimRefが欠けているPVも無視されることを確認する。
func TestParseKubernetesEngineCephRBDVolumes(t *testing.T) {
	data := []byte(`{
		"items": [
			{"metadata":{"name":"pv-rbd"},"spec":{"csi":{"driver":"rbd.csi.ceph.com","volumeAttributes":{"pool":"rbdpool","imageName":"csi-vol-1"}},"claimRef":{"namespace":"default","name":"claim-1"}}},
			{"metadata":{"name":"pv-cephfs"},"spec":{"csi":{"driver":"cephfs.csi.ceph.com","volumeAttributes":{"fsName":"cephfs","subvolumeName":"csi-vol-2"}},"claimRef":{"namespace":"default","name":"claim-2"}}},
			{"metadata":{"name":"pv-no-claim"},"spec":{"csi":{"driver":"rbd.csi.ceph.com","volumeAttributes":{"pool":"rbdpool"}}}},
			{"metadata":{"name":"pv-no-csi"},"spec":{}}
		]
	}`)

	refs, err := parseKubernetesEngineCephRBDVolumes(data)
	if err != nil {
		t.Fatalf("parseKubernetesEngineCephRBDVolumes() failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %+v, want exactly 1 entry", refs)
	}
	if refs[0].PVName != "pv-rbd" || refs[0].ClaimNamespace != "default" || refs[0].ClaimName != "claim-1" {
		t.Fatalf("refs[0] = %+v, unexpected content", refs[0])
	}
}
