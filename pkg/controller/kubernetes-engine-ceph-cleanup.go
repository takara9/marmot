package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
)

// kubernetesEngineRBDCSIDriverName は、Ceph-CSI(RBD)のCSIドライバ名。
const kubernetesEngineRBDCSIDriverName = "rbd.csi.ceph.com"

// kubernetesEngineCephRBDDeleteWaitAttempts / kubernetesEngineCephRBDDeleteWaitInterval は、
// PVC削除要求後にCeph-CSIがRBD imageを削除しPVが消えるのを待つ試行回数と待機間隔。
// watcher残留等でCeph-CSI側の削除が即時に完了しない場合があるため、複数回ポーリングする。
const (
	kubernetesEngineCephRBDDeleteWaitAttempts = 3
	kubernetesEngineCephRBDDeleteWaitInterval = 10 * time.Second
)

// kubernetesEngineCephRBDDeleteWaitSleepFn は差し替え口(単体テストで実際に待機しないため)。
var kubernetesEngineCephRBDDeleteWaitSleepFn = time.Sleep

// kubernetesEnginePersistentVolumeList は /api/v1/persistentvolumes のレスポンス解析に使う
// 最小限の構造体。
type kubernetesEnginePersistentVolumeList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			CSI *struct {
				Driver           string            `json:"driver"`
				VolumeAttributes map[string]string `json:"volumeAttributes"`
			} `json:"csi"`
			ClaimRef *struct {
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
			} `json:"claimRef"`
		} `json:"spec"`
	} `json:"items"`
}

// kubernetesEngineCephRBDVolumeRef は、削除対象のRBD-backed PVの所在(PV名・PVCの名前空間/名前)。
type kubernetesEngineCephRBDVolumeRef struct {
	PVName         string
	ClaimNamespace string
	ClaimName      string
}

// parseKubernetesEngineCephRBDVolumes は、/api/v1/persistentvolumes のレスポンスJSONから、
// Ceph-CSI(RBD)が払い出したPVのうち、対応するPVCが存在するものを抽出する。CephFSサブボリューム
// (driver=cephfs.csi.ceph.com)は対象外とする。
func parseKubernetesEngineCephRBDVolumes(data []byte) ([]kubernetesEngineCephRBDVolumeRef, error) {
	var pvList kubernetesEnginePersistentVolumeList
	if err := json.Unmarshal(data, &pvList); err != nil {
		return nil, fmt.Errorf("failed to decode persistent volume list: %w", err)
	}
	refs := make([]kubernetesEngineCephRBDVolumeRef, 0, len(pvList.Items))
	for _, item := range pvList.Items {
		if item.Spec.CSI == nil || item.Spec.CSI.Driver != kubernetesEngineRBDCSIDriverName {
			continue
		}
		if item.Spec.ClaimRef == nil {
			continue
		}
		namespace := strings.TrimSpace(item.Spec.ClaimRef.Namespace)
		name := strings.TrimSpace(item.Spec.ClaimRef.Name)
		if namespace == "" || name == "" {
			continue
		}
		refs = append(refs, kubernetesEngineCephRBDVolumeRef{
			PVName:         item.Metadata.Name,
			ClaimNamespace: namespace,
			ClaimName:      name,
		})
	}
	return refs, nil
}

// listKubernetesEngineCephRBDVolumes は、クラスタのkube-apiserverからPersistentVolume一覧を取得し、
// Ceph-CSI(RBD)が払い出したPVのうちPVCが結びついているものを返す。
func listKubernetesEngineCephRBDVolumes(ke api.KubernetesEngine) ([]kubernetesEngineCephRBDVolumeRef, error) {
	namespace, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := apiBase + "/api/v1/persistentvolumes"
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--fail", "--silent", "--show-error",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath, targetURL).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list persistent volumes: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}
	return parseKubernetesEngineCephRBDVolumes(output)
}

// listKubernetesEngineCephRBDVolumesFn は差し替え口(単体テストで実クラスタ無しに検証するため)。
var listKubernetesEngineCephRBDVolumesFn = listKubernetesEngineCephRBDVolumes

// deleteKubernetesEnginePersistentVolumeClaim は対象PVCの削除要求を発行する(kubectl delete pvc相当)。
// PVCが既に存在しない場合は完了済みとみなす。
func deleteKubernetesEnginePersistentVolumeClaim(ke api.KubernetesEngine, namespace, name string) error {
	nsNetns, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := fmt.Sprintf("%s/api/v1/namespaces/%s/persistentvolumeclaims/%s", apiBase, namespace, name)
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", nsNetns, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-X", "DELETE", targetURL).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete PVC %s/%s: %w (output=%s)", namespace, name, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	switch code {
	case "200", "202", "404":
		return nil
	default:
		return fmt.Errorf("unexpected status %s when deleting PVC %s/%s", code, namespace, name)
	}
}

// deleteKubernetesEnginePersistentVolumeClaimFn は差し替え口(単体テストで実クラスタ無しに検証するため)。
var deleteKubernetesEnginePersistentVolumeClaimFn = deleteKubernetesEnginePersistentVolumeClaim

// kubernetesEnginePodListForPVC は、対象namespaceのPod一覧のうちspec.volumes・所有者情報の解析に
// 使う最小限の構造体。
type kubernetesEnginePodListForPVC struct {
	Items []struct {
		Metadata struct {
			Name            string `json:"name"`
			OwnerReferences []struct {
				Kind       string `json:"kind"`
				Name       string `json:"name"`
				Controller *bool  `json:"controller"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Spec struct {
			Volumes []struct {
				PersistentVolumeClaim *struct {
					ClaimName string `json:"claimName"`
				} `json:"persistentVolumeClaim"`
			} `json:"volumes"`
		} `json:"spec"`
	} `json:"items"`
}

// kubernetesEnginePodOwnerRef は、Podを再作成しうる所有コントローラーの種類と名前。
type kubernetesEnginePodOwnerRef struct {
	Kind string
	Name string
}

// kubernetesEngineMountingPod は、対象PVCを参照しているPodと、その直接の所有コントローラー
// (controller=trueのownerReference、無ければnil)。
type kubernetesEngineMountingPod struct {
	Namespace string
	Name      string
	Owner     *kubernetesEnginePodOwnerRef
}

// listKubernetesEnginePodsUsingPVC は、対象PVCをspec.volumesで参照しているPodを
// 同一namespace内から列挙する(kube-apiserverはvolumeによるfieldSelectorをサポートしないため、
// namespace内の全Podを取得しクライアント側でフィルタする)。
func listKubernetesEnginePodsUsingPVC(ke api.KubernetesEngine, namespace, claimName string) ([]kubernetesEngineMountingPod, error) {
	nsNetns, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := apiBase + "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods"
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", nsNetns, "curl",
		"--fail", "--silent", "--show-error",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath, targetURL).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w (output=%s)", namespace, err, strings.TrimSpace(string(output)))
	}
	var podList kubernetesEnginePodListForPVC
	if err := json.Unmarshal(output, &podList); err != nil {
		return nil, fmt.Errorf("failed to decode pod list for namespace %s: %w", namespace, err)
	}
	pods := make([]kubernetesEngineMountingPod, 0)
	for _, item := range podList.Items {
		for _, volume := range item.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != claimName {
				continue
			}
			pod := kubernetesEngineMountingPod{Namespace: namespace, Name: item.Metadata.Name}
			for _, owner := range item.Metadata.OwnerReferences {
				if owner.Controller != nil && *owner.Controller {
					pod.Owner = &kubernetesEnginePodOwnerRef{Kind: owner.Kind, Name: owner.Name}
					break
				}
			}
			pods = append(pods, pod)
			break
		}
	}
	return pods, nil
}

// listKubernetesEnginePodsUsingPVCFn は差し替え口(単体テストで実クラスタ無しに検証するため)。
var listKubernetesEnginePodsUsingPVCFn = listKubernetesEnginePodsUsingPVC

// kubernetesEngineObjectOwnerReferences は、Deployment/ReplicaSet等の任意オブジェクトから
// ownerReferencesだけを取り出すための最小限の構造体。
type kubernetesEngineObjectOwnerReferences struct {
	Metadata struct {
		OwnerReferences []struct {
			Kind       string `json:"kind"`
			Name       string `json:"name"`
			Controller *bool  `json:"controller"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
}

// kubernetesEngineControllerAPIPath は、コントローラー種別ごとのkube-apiserver上のREST APIパスを返す。
// Ceph RBD PVCを参照しうる代表的なワークロードコントローラーのみ対応する。
func kubernetesEngineControllerAPIPath(namespace string, owner kubernetesEnginePodOwnerRef) (string, error) {
	ns := url.PathEscape(namespace)
	name := url.PathEscape(owner.Name)
	switch owner.Kind {
	case "Deployment":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", ns, name), nil
	case "ReplicaSet":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/replicasets/%s", ns, name), nil
	case "StatefulSet":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s", ns, name), nil
	case "DaemonSet":
		return fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets/%s", ns, name), nil
	case "Job":
		return fmt.Sprintf("/apis/batch/v1/namespaces/%s/jobs/%s", ns, name), nil
	default:
		return "", fmt.Errorf("unsupported controller kind %q for %s/%s", owner.Kind, namespace, owner.Name)
	}
}

// resolveKubernetesEngineTopLevelController は、PodのownerReferenceがReplicaSetの場合、
// そのReplicaSetをさらに所有するDeployment等を辿ってトップレベルのコントローラーを特定する
// (Deployment管理下のPodはownerReferenceが直接ReplicaSetを指すため)。
func resolveKubernetesEngineTopLevelController(ke api.KubernetesEngine, namespace string, owner kubernetesEnginePodOwnerRef) (kubernetesEnginePodOwnerRef, error) {
	if owner.Kind != "ReplicaSet" {
		return owner, nil
	}
	nsNetns, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return owner, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/replicasets/%s", apiBase, url.PathEscape(namespace), url.PathEscape(owner.Name))
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", nsNetns, "curl",
		"--fail", "--silent", "--show-error",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath, targetURL).CombinedOutput()
	if err != nil {
		// ReplicaSetが既に見つからない場合は、Pod自体の情報だけで強制削除を続行する。
		return owner, nil
	}
	var rs kubernetesEngineObjectOwnerReferences
	if err := json.Unmarshal(output, &rs); err != nil {
		return owner, fmt.Errorf("failed to decode replicaset %s/%s: %w", namespace, owner.Name, err)
	}
	for _, rsOwner := range rs.Metadata.OwnerReferences {
		if rsOwner.Controller != nil && *rsOwner.Controller {
			return kubernetesEnginePodOwnerRef{Kind: rsOwner.Kind, Name: rsOwner.Name}, nil
		}
	}
	return owner, nil
}

// resolveKubernetesEngineTopLevelControllerFn は差し替え口(単体テストで実クラスタ無しに検証するため)。
var resolveKubernetesEngineTopLevelControllerFn = resolveKubernetesEngineTopLevelController

// deleteKubernetesEngineController は、対象コントローラー(Deployment/ReplicaSet/StatefulSet/
// DaemonSet/Job)を削除する(kubectl delete相当)。コントローラーを消さずにPodだけ強制削除すると
// 即座に新しいPodが再作成され、PVCが再マウントされてしまうため、Pod強制削除の前に呼び出す。
// コントローラーが既に存在しない場合は完了済みとみなす。
func deleteKubernetesEngineController(ke api.KubernetesEngine, namespace string, owner kubernetesEnginePodOwnerRef) error {
	apiPath, err := kubernetesEngineControllerAPIPath(namespace, owner)
	if err != nil {
		return err
	}
	nsNetns, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := apiBase + apiPath
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", nsNetns, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-X", "DELETE", targetURL).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete %s %s/%s: %w (output=%s)", owner.Kind, namespace, owner.Name, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	switch code {
	case "200", "202", "404":
		return nil
	default:
		return fmt.Errorf("unexpected status %s when deleting %s %s/%s", code, owner.Kind, namespace, owner.Name)
	}
}

// deleteKubernetesEngineControllerFn は差し替え口(単体テストで実クラスタ無しに検証するため)。
var deleteKubernetesEngineControllerFn = deleteKubernetesEngineController

// forceDeleteKubernetesEnginePod は対象Podをgraceful終了を待たず即座に削除する
// (kubectl delete pod --force --grace-period=0相当)。ノードVMが強制電源断で消えており
// kubeletが終了を報告できないため、通常のグレースフル削除では終わらない。
// Podが既に存在しない場合は完了済みとみなす。
func forceDeleteKubernetesEnginePod(ke api.KubernetesEngine, namespace, name string) error {
	nsNetns, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s", apiBase, namespace, name)
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", nsNetns, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-H", "Content-Type: application/json",
		"-X", "DELETE", "--data-binary", "@-", targetURL)
	cmd.Stdin = strings.NewReader(`{"gracePeriodSeconds":0}`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to force delete pod %s/%s: %w (output=%s)", namespace, name, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	switch code {
	case "200", "202", "404":
		return nil
	default:
		return fmt.Errorf("unexpected status %s when force deleting pod %s/%s", code, namespace, name)
	}
}

// forceDeleteKubernetesEnginePodFn は差し替え口(単体テストで実クラスタ無しに検証するため)。
var forceDeleteKubernetesEnginePodFn = forceDeleteKubernetesEnginePod

// removeKubernetesEngineCephRBDImages は、KubernetesEngine削除時に、Ceph-CSI(RBD)が払い出し済みの
// PVCをマウント中のPodごと強制削除したうえでkube-apiserver経由でPVC削除し、RBD imageの実削除を
// Ceph-CSI本来の削除経路(watcher解放等を含む)に委ねる(CephFSサブボリュームは対象外)。ノード用VM
// (Ceph-CSIのprovisioner Podの実行元)が破棄される前の最後のタイミングで呼び出す必要がある。
// ベストエフォートとし、失敗してもクラスタ削除は継続する。
func removeKubernetesEngineCephRBDImages(ke api.KubernetesEngine) error {
	refs, err := listKubernetesEngineCephRBDVolumesFn(ke)
	if err != nil {
		return fmt.Errorf("list persistent volumes: %w", err)
	}
	if len(refs) == 0 {
		return nil
	}

	pending := make(map[string]kubernetesEngineCephRBDVolumeRef, len(refs))
	var errs []error
	for _, ref := range refs {
		// マウント中のPodが残っているとpvc-protection finalizerでPVC削除がブロックされ、
		// Ceph-CSI側のRBD image削除が起動しないため、PVC削除前にPodを強制削除する。
		// kube-systemはシステムコンポーネントが稼働するため強制削除の対象から除外する。
		if ref.ClaimNamespace != "kube-system" {
			pods, podErr := listKubernetesEnginePodsUsingPVCFn(ke, ref.ClaimNamespace, ref.ClaimName)
			if podErr != nil {
				errs = append(errs, fmt.Errorf("list pods using PVC %s/%s: %w", ref.ClaimNamespace, ref.ClaimName, podErr))
			} else {
				// Pod強制削除だけではコントローラーが即座に代替Podを作りPVCを再マウントするため、
				// 先に所有コントローラーを削除する(同一コントローラーへの重複削除は避ける)。
				deletedControllers := make(map[string]bool)
				for _, pod := range pods {
					if pod.Owner != nil {
						topOwner, resolveErr := resolveKubernetesEngineTopLevelControllerFn(ke, ref.ClaimNamespace, *pod.Owner)
						if resolveErr != nil {
							errs = append(errs, fmt.Errorf("resolve controller for pod %s/%s (pvc=%s/%s): %w", pod.Namespace, pod.Name, ref.ClaimNamespace, ref.ClaimName, resolveErr))
						} else {
							key := topOwner.Kind + "/" + topOwner.Name
							if !deletedControllers[key] {
								deletedControllers[key] = true
								if err := deleteKubernetesEngineControllerFn(ke, ref.ClaimNamespace, topOwner); err != nil {
									errs = append(errs, fmt.Errorf("delete controller %s %s/%s (pvc=%s/%s): %w", topOwner.Kind, ref.ClaimNamespace, topOwner.Name, ref.ClaimNamespace, ref.ClaimName, err))
								}
							}
						}
					}
					if err := forceDeleteKubernetesEnginePodFn(ke, pod.Namespace, pod.Name); err != nil {
						errs = append(errs, fmt.Errorf("force delete pod %s/%s (pvc=%s/%s): %w", pod.Namespace, pod.Name, ref.ClaimNamespace, ref.ClaimName, err))
					}
				}
			}
		}
		if err := deleteKubernetesEnginePersistentVolumeClaimFn(ke, ref.ClaimNamespace, ref.ClaimName); err != nil {
			errs = append(errs, fmt.Errorf("delete PVC %s/%s (pv=%s): %w", ref.ClaimNamespace, ref.ClaimName, ref.PVName, err))
			continue
		}
		pending[ref.PVName] = ref
	}

	for attempt := 0; len(pending) > 0 && attempt < kubernetesEngineCephRBDDeleteWaitAttempts; attempt++ {
		kubernetesEngineCephRBDDeleteWaitSleepFn(kubernetesEngineCephRBDDeleteWaitInterval)
		remaining, listErr := listKubernetesEngineCephRBDVolumesFn(ke)
		if listErr != nil {
			errs = append(errs, fmt.Errorf("re-list persistent volumes: %w", listErr))
			break
		}
		stillPresent := make(map[string]struct{}, len(remaining))
		for _, r := range remaining {
			stillPresent[r.PVName] = struct{}{}
		}
		for pvName := range pending {
			if _, ok := stillPresent[pvName]; !ok {
				delete(pending, pvName)
			}
		}
	}

	for _, ref := range pending {
		errs = append(errs, fmt.Errorf("RBD-backed PV %s (pvc=%s/%s) still present after PVC deletion", ref.PVName, ref.ClaimNamespace, ref.ClaimName))
	}

	return errors.Join(errs...)
}
