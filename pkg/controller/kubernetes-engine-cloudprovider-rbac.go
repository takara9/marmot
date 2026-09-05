package controller

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
)

// kubernetesEngineCloudControllerManagerRBACProbeURLPath は、CCM用RBACが既に適用済みかどうかの
// 判定に使うClusterRoleのURLパス。
const kubernetesEngineCloudControllerManagerRBACProbeURLPath = "/apis/rbac.authorization.k8s.io/v1/clusterroles/system:cloud-controller-manager"

// EnsureKubernetesEngineCloudControllerManagerRBAC は、cloud-controller-manager
// (クライアント証明書のCommonName/Organization = system:cloud-controller-manager) が
// kube-apiserverのnodes/servicesへアクセスするために必要なClusterRole/ClusterRoleBindingを
// コントロールプレーンのAPIサーバーへ適用する。Kubernetes組み込みの同名ClusterRoleBindingは
// 存在しないため、CoreDNS(EnsureKubernetesEngineClusterDNS)と同じ方式で明示的に用意する。
// 既に適用済みの場合は何もしない（冪等）。EnsureKubernetesEngineClusterDNS同様、コントロール
// プレーンのAPIサーバーが到達可能であることを前提とするため、ノードプロビジョニング時に呼び出すこと。
func EnsureKubernetesEngineCloudControllerManagerRBAC(ke api.KubernetesEngine) error {
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return err
	}
	caPath, _, err := EnsureKubernetesEngineCA(DefaultKubernetesEnginePkiDir, clusterName)
	if err != nil {
		return err
	}
	adminCertPath, adminKeyPath, err := IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "controller-admin",
		CommonName:    "mke-controller-admin",
		Organizations: []string{"system:masters"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return err
	}
	apiEndpointBase := fmt.Sprintf("https://%s:%d", *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)

	installed, err := kubernetesEngineAPIResourceExists(namespace, caPath, adminCertPath, adminKeyPath,
		apiEndpointBase+kubernetesEngineCloudControllerManagerRBACProbeURLPath)
	if err != nil {
		return err
	}
	if installed {
		return nil
	}

	for _, doc := range kubernetesEngineCloudControllerManagerRBACManifests() {
		if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, []byte(doc)); err != nil {
			return err
		}
	}
	return nil
}

// kubernetesEngineCloudControllerManagerRBACManifests は、cloud-controller-managerの
// クライアント証明書のUser識別子(kubernetesEngineCloudControllerManagerUserName)にnodes/services
// の読み書き権限を与えるClusterRole/ClusterRoleBindingを返す。付与する権限は
// cmd/mke-node-controller/reconcile.goとkubeclient.goが実際に呼び出すAPI
// (GET /api/v1/nodes, PATCH /api/v1/nodes/{name}(/status), GET /api/v1/services,
// PATCH /api/v1/namespaces/{ns}/services/{name}/status)に必要な最小限に絞っている。
func kubernetesEngineCloudControllerManagerRBACManifests() []string {
	return []string{
		kubernetesEngineCloudControllerManagerClusterRoleYAML(),
		kubernetesEngineCloudControllerManagerClusterRoleBindingYAML(),
	}
}

func kubernetesEngineCloudControllerManagerClusterRoleYAML() string {
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: system:cloud-controller-manager
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "patch", "update"]
  - apiGroups: [""]
    resources: ["nodes/status"]
    verbs: ["patch", "update"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["services/status"]
    verbs: ["patch", "update"]
`
}

func kubernetesEngineCloudControllerManagerClusterRoleBindingYAML() string {
	return fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system:cloud-controller-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:cloud-controller-manager
subjects:
  - kind: User
    name: %s
    apiGroup: rbac.authorization.k8s.io
`, kubernetesEngineCloudControllerManagerUserName)
}
