package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takara9/marmot/api"
)

// kubernetesEngineMetricsServerManifestsSubdir は、DefaultKubernetesEngineMKEManifestsDir配下で
// metrics-serverインストールマニフェスト(mke/metrics-server由来)が置かれるディレクトリ名。
const kubernetesEngineMetricsServerManifestsSubdir = "metrics-server"

// kubernetesEngineMetricsServerManifestFile は、metrics-serverマニフェストのファイル名
// (mke/metrics-server/README.md 記載の upstream components.yaml をそのまま配置したもの)。
const kubernetesEngineMetricsServerManifestFile = "components.yaml"

// kubernetesEngineMetricsServerProbeURLPath は、metrics-serverが既にインストール済みかどうかの
// 判定に使うDeploymentのURLパス。
const kubernetesEngineMetricsServerProbeURLPath = "/apis/apps/v1/namespaces/kube-system/deployments/metrics-server"

// EnsureKubernetesEngineMetricsServer は、DefaultKubernetesEngineMKEManifestsDir/metrics-server
// 配下のマニフェストをコントロールプレーンのAPIサーバーへ適用する。metrics-serverはAPI集約層
// (aggregation layer)を利用するため、kube-apiserver側の--requestheader-*/--proxy-client-*/
// --enable-aggregator-routing設定(EnsureKubernetesEngineControlPlaneAssets/
// renderKubernetesEngineControlPlaneUnitsで付与済み)が前提となる。
// 既にmetrics-serverのDeploymentが存在する場合は何もしない（冪等）。
func EnsureKubernetesEngineMetricsServer(ke api.KubernetesEngine) error {
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	manifestPath := filepath.Join(DefaultKubernetesEngineMKEManifestsDir, kubernetesEngineMetricsServerManifestsSubdir, kubernetesEngineMetricsServerManifestFile)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read metrics-server manifest %s: %w", manifestPath, err)
	}
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return err
	}
	caPath, _ := KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, clusterName)
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
		apiEndpointBase+kubernetesEngineMetricsServerProbeURLPath)
	if err != nil {
		return err
	}
	if installed {
		return nil
	}

	for _, doc := range splitKubernetesEngineYAMLDocuments(content) {
		if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
			return fmt.Errorf("failed to apply metrics-server manifest: %w", err)
		}
	}
	return nil
}
