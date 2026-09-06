package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
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
	"rbac.authorization.k8s.io/v1/ClusterRole":         {group: "rbac.authorization.k8s.io", version: "v1", resource: "clusterroles", namespaced: false},
	"rbac.authorization.k8s.io/v1/ClusterRoleBinding":  {group: "rbac.authorization.k8s.io", version: "v1", resource: "clusterrolebindings", namespaced: false},
	"rbac.authorization.k8s.io/v1/Role":                {group: "rbac.authorization.k8s.io", version: "v1", resource: "roles", namespaced: true},
	"rbac.authorization.k8s.io/v1/RoleBinding":         {group: "rbac.authorization.k8s.io", version: "v1", resource: "rolebindings", namespaced: true},
	"apiextensions.k8s.io/v1/CustomResourceDefinition": {group: "apiextensions.k8s.io", version: "v1", resource: "customresourcedefinitions", namespaced: false},
	"policy/v1/PodDisruptionBudget":                    {group: "policy", version: "v1", resource: "poddisruptionbudgets", namespaced: true},
	"storage.k8s.io/v1/StorageClass":                   {group: "storage.k8s.io", version: "v1", resource: "storageclasses", namespaced: false},
	"storage.k8s.io/v1/CSIDriver":                      {group: "storage.k8s.io", version: "v1", resource: "csidrivers", namespaced: false},
	"batch/v1/Job":                                     {group: "batch", version: "v1", resource: "jobs", namespaced: true},
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

type kubernetesEngineCiliumTLSMaterials struct {
	caCert     string
	caKey      string
	serverCert string
	serverKey  string
}

// kubernetesEngineCiliumProbeURLPath は、Ciliumが既にインストール済みかどうかの判定に使う
// DaemonSetのURLパス。
const kubernetesEngineCiliumProbeURLPath = "/apis/apps/v1/namespaces/kube-system/daemonsets/cilium"

// kubernetesEngineCiliumManifestsSubdir は、DefaultKubernetesEngineMKEManifestsDir配下で
// Ciliumインストールマニフェスト(mke/cni-cilium由来)が置かれるディレクトリ名。
const kubernetesEngineCiliumManifestsSubdir = "cni-cilium"

// EnsureKubernetesEngineCiliumCNI は spec.nodeSpec.network.cni-plugin=cilium のクラスタに対して、
// DefaultKubernetesEngineMKEManifestsDir/cni-cilium 配下のYAMLマニフェスト群を
// コントロールプレーンのAPIサーバーへ適用する。既にCiliumのDaemonSetが存在する場合は
// 何もしない（冪等）。
//
// 制約: マニフェストの適用は「存在しなければ作成する」のみを行い、既存リソースの
// 更新（kubectl applyのようなdiffベースの更新）は行わない。バージョンアップ等で
// マニフェストの内容を更新したい場合は、既存リソースを削除してから再実行すること。
func EnsureKubernetesEngineCiliumCNI(mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) error {
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	manifestDir := filepath.Join(DefaultKubernetesEngineMKEManifestsDir, kubernetesEngineCiliumManifestsSubdir)
	manifestFiles, err := kubernetesEngineCiliumManifestFiles(manifestDir)
	if err != nil {
		return err
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

	tlsMaterials, err := generateKubernetesEngineCiliumTLSMaterials()
	if err != nil {
		return err
	}
	for _, name := range manifestFiles {
		content, err := os.ReadFile(filepath.Join(manifestDir, name))
		if err != nil {
			return fmt.Errorf("failed to read Cilium manifest %s: %w", name, err)
		}
		for _, doc := range splitKubernetesEngineYAMLDocuments(content) {
			doc, err = prepareKubernetesEngineCiliumManifest(doc, tlsMaterials)
			if err != nil {
				return fmt.Errorf("failed to prepare Cilium manifest %s: %w", name, err)
			}
			if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
				return fmt.Errorf("failed to apply Cilium manifest %s: %w", name, err)
			}
		}
	}
	return nil
}

func generateKubernetesEngineCiliumTLSMaterials() (kubernetesEngineCiliumTLSMaterials, error) {
	now := time.Now().UTC()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return kubernetesEngineCiliumTLSMaterials{}, err
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return kubernetesEngineCiliumTLSMaterials{}, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "cilium-ca"},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return kubernetesEngineCiliumTLSMaterials{}, err
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return kubernetesEngineCiliumTLSMaterials{}, err
	}
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return kubernetesEngineCiliumTLSMaterials{}, err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "hubble-grpc.cilium.io"},
		DNSNames:     []string{"*.hubble-grpc.cilium.io", "*.default.hubble-grpc.cilium.io"},
		NotBefore:    now,
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return kubernetesEngineCiliumTLSMaterials{}, err
	}

	encode := func(typ string, data []byte) string {
		return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: data}))
	}
	return kubernetesEngineCiliumTLSMaterials{
		caCert:     encode("CERTIFICATE", caDER),
		caKey:      encode("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(caKey)),
		serverCert: encode("CERTIFICATE", serverDER),
		serverKey:  encode("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey)),
	}, nil
}

func prepareKubernetesEngineCiliumManifest(doc []byte, materials kubernetesEngineCiliumTLSMaterials) ([]byte, error) {
	var obj kubernetesEngineManifestObject
	if err := yaml.Unmarshal(doc, &obj); err != nil {
		return nil, err
	}
	if obj.Kind != "Secret" || (obj.Metadata.Name != "cilium-ca" && obj.Metadata.Name != "hubble-server-certs") {
		return doc, nil
	}
	var manifest map[string]interface{}
	if err := yaml.Unmarshal(doc, &manifest); err != nil {
		return nil, err
	}
	if obj.Metadata.Name == "cilium-ca" {
		manifest["data"] = map[string]string{"ca.crt": materials.caCert, "ca.key": materials.caKey}
	} else {
		manifest["data"] = map[string]string{
			"ca.crt": materials.caCert, "tls.crt": materials.serverCert, "tls.key": materials.serverKey,
		}
	}
	return yaml.Marshal(manifest)
}

// kubernetesEngineCiliumManifestFiles は、dir直下にある".yaml"/".yml"拡張子のファイル名を
// (os.ReadDirの仕様によりファイル名順で)返す。隠しファイル(vimのスワップファイル等)や
// サブディレクトリは無視する。1件も無い場合はエラーとする。
func kubernetesEngineCiliumManifestFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read Cilium manifests dir %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		files = append(files, entry.Name())
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Cilium manifest files found in %s", dir)
	}
	return files, nil
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
	code := strings.TrimSpace(string(output))
	switch code {
	case "200":
		return true, nil
	case "404":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status %s when checking existing resource at %s", code, url)
	}
}
