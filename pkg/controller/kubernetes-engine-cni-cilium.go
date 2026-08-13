package controller

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
	"gopkg.in/yaml.v3"
)

// kubernetesEngineManifestResourceMeta は、Kindごとにkube-apiserverのREST APIパスを
// 組み立てるための情報を保持する。
type kubernetesEngineManifestResourceMeta struct {
	group      string // 空文字はcore(v1) APIグループ
	version    string
	resource   string // 複数形のリソース名
	namespaced bool
}

// kubernetesEngineManifestResources は、Ciliumのインストールマニフェストに含まれ得る
// リソース種別のうち、対応済みのものの一覧。ここに無いKindのマニフェストが渡された
// 場合は、サイレントに無視せずエラーとする。
var kubernetesEngineManifestResources = map[string]kubernetesEngineManifestResourceMeta{
	"v1/Namespace":       {version: "v1", resource: "namespaces", namespaced: false},
	"v1/ServiceAccount":  {version: "v1", resource: "serviceaccounts", namespaced: true},
	"v1/ConfigMap":       {version: "v1", resource: "configmaps", namespaced: true},
	"v1/Secret":          {version: "v1", resource: "secrets", namespaced: true},
	"v1/Service":         {version: "v1", resource: "services", namespaced: true},
	"apps/v1/DaemonSet":  {group: "apps", version: "v1", resource: "daemonsets", namespaced: true},
	"apps/v1/Deployment": {group: "apps", version: "v1", resource: "deployments", namespaced: true},
	"rbac.authorization.k8s.io/v1/ClusterRole":        {group: "rbac.authorization.k8s.io", version: "v1", resource: "clusterroles", namespaced: false},
	"rbac.authorization.k8s.io/v1/ClusterRoleBinding": {group: "rbac.authorization.k8s.io", version: "v1", resource: "clusterrolebindings", namespaced: false},
	"rbac.authorization.k8s.io/v1/Role":               {group: "rbac.authorization.k8s.io", version: "v1", resource: "roles", namespaced: true},
	"rbac.authorization.k8s.io/v1/RoleBinding":        {group: "rbac.authorization.k8s.io", version: "v1", resource: "rolebindings", namespaced: true},
	"apiextensions.k8s.io/v1/CustomResourceDefinition": {group: "apiextensions.k8s.io", version: "v1", resource: "customresourcedefinitions", namespaced: false},
	"policy/v1/PodDisruptionBudget":                     {group: "policy", version: "v1", resource: "poddisruptionbudgets", namespaced: true},
}

// kubernetesEngineManifestObject は、マニフェスト中の1ドキュメントからkind/name/namespaceを
// 読み取るための最小限の構造体。
type kubernetesEngineManifestObject struct {
	ApiVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

const kubernetesEngineCiliumNamespace = "kube-system"

// kubernetesEngineCiliumProbeURLPath は、Ciliumが既にインストール済みかどうかの判定に使う
// DaemonSetのURLパス。
const kubernetesEngineCiliumProbeURLPath = "/apis/apps/v1/namespaces/kube-system/daemonsets/cilium"

// EnsureKubernetesEngineCiliumCNI は spec.nodeSpec.network.kind=cilium のクラスタに対して、
// mkeConf.CiliumManifestURL で指定されたKubernetesインストールマニフェストを
// コントロールプレーンのAPIサーバーへ適用する。既にCiliumのDaemonSetが存在する場合は
// 何もしない（冪等）。
//
// 制約: マニフェストの適用は「存在しなければ作成する」のみを行い、既存リソースの
// 更新（kubectl applyのようなdiffベースの更新）は行わない。バージョンアップ等で
// マニフェストの内容を更新したい場合は、既存リソースを削除してから再実行すること。
func EnsureKubernetesEngineCiliumCNI(mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) error {
	manifestURL := strings.TrimSpace(mkeConf.CiliumManifestURL)
	if manifestURL == "" {
		return fmt.Errorf("nodeSpec.network.kind=cilium requires cilium_manifest_url to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
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
		apiEndpointBase+kubernetesEngineCiliumProbeURLPath)
	if err != nil {
		return err
	}
	if installed {
		return nil
	}

	manifest, err := kubernetesDownload(manifestURL)
	if err != nil {
		return fmt.Errorf("failed to download Cilium manifest from %s: %w", manifestURL, err)
	}
	for _, doc := range splitKubernetesEngineYAMLDocuments(manifest) {
		if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
			return err
		}
	}
	return nil
}

// splitKubernetesEngineYAMLDocuments は、"---"区切りのマニフェストを個々のYAMLドキュメントへ
// 分割する。各ドキュメントはkube-apiserverへそのまま送信するため、元のテキストを維持する。
func splitKubernetesEngineYAMLDocuments(data []byte) [][]byte {
	lines := strings.Split(string(data), "\n")
	var docs [][]byte
	var current []string
	flush := func() {
		joined := strings.TrimSpace(strings.Join(current, "\n"))
		if joined != "" {
			docs = append(docs, []byte(joined))
		}
		current = nil
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return docs
}

// applyKubernetesEngineManifestObject は、1つのYAMLドキュメントが表すリソースが
// 存在しなければkube-apiserverへ作成する（既に存在する場合は何もしない）。
func applyKubernetesEngineManifestObject(namespace, caPath, certPath, keyPath, apiEndpointBase string, doc []byte) error {
	var obj kubernetesEngineManifestObject
	if err := yaml.Unmarshal(doc, &obj); err != nil {
		return fmt.Errorf("failed to parse manifest document: %w", err)
	}
	if strings.TrimSpace(obj.Kind) == "" {
		// 空ドキュメントやコメントのみのドキュメントはスキップ
		return nil
	}
	key := strings.TrimSpace(obj.ApiVersion) + "/" + strings.TrimSpace(obj.Kind)
	meta, ok := kubernetesEngineManifestResources[key]
	if !ok {
		return fmt.Errorf("unsupported manifest resource %q; add it to kubernetesEngineManifestResources to support installing it", key)
	}
	if strings.TrimSpace(obj.Metadata.Name) == "" {
		return fmt.Errorf("manifest resource %s is missing metadata.name", key)
	}

	urlPath := "/api/" + meta.version
	if meta.group != "" {
		urlPath = "/apis/" + meta.group + "/" + meta.version
	}
	if meta.namespaced {
		ns := strings.TrimSpace(obj.Metadata.Namespace)
		if ns == "" {
			ns = kubernetesEngineCiliumNamespace
		}
		urlPath += "/namespaces/" + ns
	}
	urlPath += "/" + meta.resource

	exists, err := kubernetesEngineAPIResourceExists(namespace, caPath, certPath, keyPath,
		apiEndpointBase+urlPath+"/"+obj.Metadata.Name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--fail", "--silent", "--show-error",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath,
		"-H", "Content-Type: application/yaml",
		"-X", "POST", "--data-binary", "@-",
		apiEndpointBase+urlPath)
	cmd.Stdin = bytes.NewReader(doc)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create %s %q: %w (output=%s)", key, obj.Metadata.Name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// kubernetesEngineAPIResourceExists は、コントロールプレーンのAPIサーバー上に
// 指定URLのリソースが既に存在するかどうかをGETのHTTPステータスコードで判定する。
func kubernetesEngineAPIResourceExists(namespace, caPath, certPath, keyPath, url string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl",
		"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}",
		"--cacert", caPath, "--cert", certPath, "--key", keyPath, url).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check existing resource at %s: %w (output=%s)", url, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) == "200", nil
}
