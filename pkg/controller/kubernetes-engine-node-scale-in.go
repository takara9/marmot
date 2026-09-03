package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

// kubernetesEnginePodRef はドレイン対象Podの識別に必要な最小限の情報。
type kubernetesEnginePodRef struct {
	Namespace string
	Name      string
}

// kubernetesEnginePodList は /api/v1/pods (fieldSelector付き) のレスポンス解析に使う最小限の構造体。
type kubernetesEnginePodList struct {
	Items []struct {
		Metadata struct {
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			OwnerReferences []struct {
				Kind string `json:"kind"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
	} `json:"items"`
}

// selectKubernetesEngineNodeForScaleIn は、DeletionTimeStamp未設定のアクティブノードが
// spec.nodesを超過している場合に、命名規則の番号(index)が最大のノードを1台選ぶ。
// 超過していなければ nil, nil を返す。
func selectKubernetesEngineNodeForScaleIn(database *db.Database, ke api.KubernetesEngine) (*api.Server, error) {
	servers, err := findKubernetesEngineNodeServers(database, ke)
	if err != nil {
		return nil, err
	}
	active := make([]api.Server, 0, len(servers))
	for _, server := range servers {
		if server.Status == nil || server.Status.DeletionTimeStamp == nil {
			active = append(active, server)
		}
	}
	if len(active) <= ke.Spec.Nodes {
		return nil, nil
	}
	sort.Slice(active, func(i, j int) bool {
		indexI, errI := kubernetesEngineNodeIndex(active[i].Metadata.Name)
		indexJ, errJ := kubernetesEngineNodeIndex(active[j].Metadata.Name)
		if errI != nil || errJ != nil {
			return active[i].Metadata.Name > active[j].Metadata.Name
		}
		return indexI > indexJ
	})
	target := active[0]
	return &target, nil
}

// kubernetesEngineAdminAPIContext は、クラスタ専用CAで発行したcontroller-admin証明書と
// コントロールプレーンnetnsの名前、APIサーバーのベースURLをまとめて返す。
func kubernetesEngineAdminAPIContext(ke api.KubernetesEngine) (namespace, caPath, certPath, keyPath, apiBase string, err error) {
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return "", "", "", "", "", fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	namespace, _, _, err = KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return "", "", "", "", "", err
	}
	certPath, keyPath, err = IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "controller-admin",
		CommonName:    "mke-controller-admin",
		Organizations: []string{"system:masters"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return "", "", "", "", "", err
	}
	caPath, _ = KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, clusterName)
	apiBase = fmt.Sprintf("https://%s:%d", *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)
	return namespace, caPath, certPath, keyPath, apiBase, nil
}

// cordonKubernetesEngineNode は対象ノードのspec.unschedulableをtrueにする(kubectl cordon相当)。
// ノードが既に存在しない場合は完了済みとみなす。
func cordonKubernetesEngineNode(ke api.KubernetesEngine, nodeName string) error {
	namespace, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := apiBase + "/api/v1/nodes/" + nodeName
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-H", "Content-Type: application/merge-patch+json",
		"-X", "PATCH", "--data-binary", "@-", targetURL)
	cmd.Stdin = strings.NewReader(`{"spec":{"unschedulable":true}}`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to cordon node %s: %w (output=%s)", nodeName, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	if code == "200" || code == "404" {
		return nil
	}
	return fmt.Errorf("unexpected status %s when cordoning node %s", code, nodeName)
}

// uncordonKubernetesEngineNode は対象ノードのspec.unschedulableをfalseに戻す(kubectl uncordon相当)。
// ノードが既に存在しない場合は完了済みとみなす。
func uncordonKubernetesEngineNode(ke api.KubernetesEngine, nodeName string) error {
	namespace, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := apiBase + "/api/v1/nodes/" + nodeName
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-H", "Content-Type: application/merge-patch+json",
		"-X", "PATCH", "--data-binary", "@-", targetURL)
	cmd.Stdin = strings.NewReader(`{"spec":{"unschedulable":false}}`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to uncordon node %s: %w (output=%s)", nodeName, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	if code == "200" || code == "404" {
		return nil
	}
	return fmt.Errorf("unexpected status %s when uncordoning node %s", code, nodeName)
}

// listKubernetesEnginePodsOnNode は対象ノード上のPodのうち、DaemonSetが管理するもの
// (kubectl drain --ignore-daemonsets相当)を除いた一覧を返す。
func listKubernetesEnginePodsOnNode(ke api.KubernetesEngine, nodeName string) ([]kubernetesEnginePodRef, error) {
	namespace, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	query := url.Values{}
	query.Set("fieldSelector", "spec.nodeName="+nodeName)
	targetURL := apiBase + "/api/v1/pods?" + query.Encode()
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--fail", "--silent", "--show-error",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath, targetURL).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list pods on node %s: %w (output=%s)", nodeName, err, strings.TrimSpace(string(output)))
	}
	var podList kubernetesEnginePodList
	if err := json.Unmarshal(output, &podList); err != nil {
		return nil, fmt.Errorf("failed to decode pod list for node %s: %w", nodeName, err)
	}
	refs := make([]kubernetesEnginePodRef, 0, len(podList.Items))
	for _, item := range podList.Items {
		isDaemonSet := false
		for _, owner := range item.Metadata.OwnerReferences {
			if owner.Kind == "DaemonSet" {
				isDaemonSet = true
				break
			}
		}
		if isDaemonSet {
			continue
		}
		refs = append(refs, kubernetesEnginePodRef{Namespace: item.Metadata.Namespace, Name: item.Metadata.Name})
	}
	return refs, nil
}

// evictKubernetesEnginePodFn はPod退避処理の差し替え口(単体テストで実際のクラスタ無しに検証するため)。
var evictKubernetesEnginePodFn = evictKubernetesEnginePod

// evictKubernetesEnginePod は対象PodにEviction APIを発行する(kubectl drain相当)。
// Podが既に消えている場合は完了済みとみなす。
func evictKubernetesEnginePod(ke api.KubernetesEngine, namespace, name string) error {
	nsNetns, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s/eviction", apiBase, namespace, name)
	body := fmt.Sprintf(`{"apiVersion":"policy/v1","kind":"Eviction","metadata":{"name":%q,"namespace":%q}}`, name, namespace)
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", nsNetns, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-H", "Content-Type: application/json",
		"-X", "POST", "--data-binary", "@-", targetURL)
	cmd.Stdin = strings.NewReader(body)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to evict pod %s/%s: %w (output=%s)", namespace, name, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	switch code {
	case "200", "201", "404":
		return nil
	default:
		return fmt.Errorf("unexpected status %s when evicting pod %s/%s", code, namespace, name)
	}
}

// deleteKubernetesEngineNodeObject はKubernetes上のNodeオブジェクトを削除する(kubectl delete node相当)。
// ノードが既に存在しない場合は完了済みとみなす。
func deleteKubernetesEngineNodeObject(ke api.KubernetesEngine, nodeName string) error {
	namespace, caPath, certPath, keyPath, apiBase, err := kubernetesEngineAdminAPIContext(ke)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetURL := apiBase + "/api/v1/nodes/" + nodeName
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-X", "DELETE", targetURL).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete node %s: %w (output=%s)", nodeName, err, strings.TrimSpace(string(output)))
	}
	code := strings.TrimSpace(string(output))
	if code == "200" || code == "404" {
		return nil
	}
	return fmt.Errorf("unexpected status %s when deleting node %s", code, nodeName)
}

// drainKubernetesEngineNode は対象ノードをcordon→drain(DaemonSet管理下のPodを除きEviction APIで
// 退避)まで進める。ノードオブジェクト自体の削除は行わない(スケールインとローリング
// アップグレードの双方から共有するため、削除処理は呼び出し元に委ねる)。
func drainKubernetesEngineNode(ke api.KubernetesEngine, nodeName string) error {
	if err := cordonKubernetesEngineNode(ke, nodeName); err != nil {
		return err
	}
	pods, err := listKubernetesEnginePodsOnNode(ke, nodeName)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		if err := evictKubernetesEnginePodFn(ke, pod.Namespace, pod.Name); err != nil {
			return fmt.Errorf("failed to drain node %s: %w", nodeName, err)
		}
	}
	return nil
}

// DrainAndDeleteKubernetesEngineNode は対象ノードをcordon→drain(DaemonSet管理下のPodを除き
// Eviction APIで退避)→Kubernetes Node削除まで進める。VM(仮想サーバー)自体の削除要求は
// 呼び出し元(コントローラー)がSetDeleteTimestampで別途行う。
func DrainAndDeleteKubernetesEngineNode(ke api.KubernetesEngine, nodeName string) error {
	if err := drainKubernetesEngineNode(ke, nodeName); err != nil {
		return err
	}
	if err := deleteKubernetesEngineNodeObject(ke, nodeName); err != nil {
		return err
	}
	return nil
}
