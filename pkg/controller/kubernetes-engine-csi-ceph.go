package controller

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/ceph"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	// kubernetesEngineCephCSIClusterID は ceph-csi の ConfigMap/StorageClass が参照する
	// clusterID。marmotは単一のCephクラスタとの連携のみを前提とするため固定値でよい。
	kubernetesEngineCephCSIClusterID = "marmot-ceph"

	kubernetesEngineCephCSIRBDProvisioner    = "rbd.csi.ceph.com"
	kubernetesEngineCephCSICephFSProvisioner = "cephfs.csi.ceph.com"

	kubernetesEngineCephCSIConfigMapName    = "ceph-csi-config"
	kubernetesEngineCephCSIRBDSecretName    = "csi-rbd-secret"
	kubernetesEngineCephCSICephFSSecretName = "csi-cephfs-secret"

	// kubernetesEngineCephCSIDefaultRBDStorageClass は、ceph_pool_by_class に
	// このキーが存在する場合にのみ、既定StorageClassとしてマークする対象。
	kubernetesEngineCephCSIDefaultRBDStorageClass = "hdd"

	// kubernetesEngineCephCSIRBDProbeURLPath / kubernetesEngineCephCSICephFSProbeURLPath は、
	// 各ドライバーが既にインストール済みかどうかの判定に使うDeploymentのURLパス。
	kubernetesEngineCephCSIRBDProbeURLPath    = "/apis/apps/v1/namespaces/kube-system/deployments/csi-rbdplugin-provisioner"
	kubernetesEngineCephCSICephFSProbeURLPath = "/apis/apps/v1/namespaces/kube-system/deployments/csi-cephfsplugin-provisioner"
)

var kubernetesEngineCephCSIKeyringKeyPattern = regexp.MustCompile(`(?i)^\s*key\s*=\s*(\S+)\s*$`)

// EnsureKubernetesEngineCephCSI は、marmotd.json の ceph_enabled=true の場合に、
// Ceph CSI（RBD/ブロックストレージ用、CephFS/ファイルストレージ用の2ドライバー）を
// コントロールプレーンのAPIサーバーへ適用する。Cephが無効な場合は何もしない（no-op）。
//
// 制約: マニフェストの適用は「存在しなければ作成する」のみを行い、既存リソースの
// 更新は行わない。ceph_pool_by_class等の設定を変更した場合、反映するには既存の
// ConfigMap/Secret/StorageClassを削除してから再実行すること。
func EnsureKubernetesEngineCephCSI(mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) error {
	cephCfg := marmotd.CurrentConfig()
	if !cephCfg.CephEnabled {
		return nil
	}
	rbdManifestURL := strings.TrimSpace(mkeConf.CephCSIRBDManifestURL)
	if rbdManifestURL == "" {
		return fmt.Errorf("ceph_enabled=true requires ceph_csi_rbd_manifest_url to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	if !strings.HasPrefix(rbdManifestURL, "https://") {
		return fmt.Errorf("ceph_csi_rbd_manifest_url must use https:// (got %q)", rbdManifestURL)
	}
	cephfsManifestURL := strings.TrimSpace(mkeConf.CephCSICephFSManifestURL)
	if cephfsManifestURL == "" {
		return fmt.Errorf("ceph_enabled=true requires ceph_csi_cephfs_manifest_url to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	if !strings.HasPrefix(cephfsManifestURL, "https://") {
		return fmt.Errorf("ceph_csi_cephfs_manifest_url must use https:// (got %q)", cephfsManifestURL)
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

	monitors, user, err := ceph.ParseConnectionFromConf(marmotd.DefaultCephConfPath, marmotd.DefaultCephKeyringPath)
	if err != nil {
		return fmt.Errorf("failed to read Ceph connection info: %w", err)
	}
	keyringContent, err := os.ReadFile(marmotd.DefaultCephKeyringPath)
	if err != nil {
		return fmt.Errorf("failed to read Ceph keyring: %w", err)
	}
	keyValue, err := kubernetesEngineCephCSIKeyFromKeyring(keyringContent)
	if err != nil {
		return fmt.Errorf("failed to parse Ceph keyring: %w", err)
	}

	apply := func(manifest []byte) error {
		// Create cluster-specific resources first so upstream manifests don't create placeholders
		// that would later prevent these from being created (apply is create-only).
		for _, doc := range kubernetesEngineCephCSIGeneratedManifests(cephCfg.CephPoolByClass, monitors, user, keyValue, strings.TrimSpace(mkeConf.CephFilesystemName)) {
			if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
				return err
			}
		}
		for _, doc := range splitKubernetesEngineYAMLDocuments(manifest) {
			if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
				return err
			}
		}
		return nil
	}

	rbdInstalled, err := kubernetesEngineAPIResourceExists(namespace, caPath, adminCertPath, adminKeyPath,
		apiEndpointBase+kubernetesEngineCephCSIRBDProbeURLPath)
	if err != nil {
		return err
	}
	if !rbdInstalled {
		manifest, downloadErr := kubernetesDownload(rbdManifestURL)
		if downloadErr != nil {
			return fmt.Errorf("failed to download Ceph CSI RBD manifest from %s: %w", rbdManifestURL, downloadErr)
		}
		if err := apply(manifest); err != nil {
			return err
		}
	}

	cephfsInstalled, err := kubernetesEngineAPIResourceExists(namespace, caPath, adminCertPath, adminKeyPath,
		apiEndpointBase+kubernetesEngineCephCSICephFSProbeURLPath)
	if err != nil {
		return err
	}
	if !cephfsInstalled {
		manifest, downloadErr := kubernetesDownload(cephfsManifestURL)
		if downloadErr != nil {
			return fmt.Errorf("failed to download Ceph CSI CephFS manifest from %s: %w", cephfsManifestURL, downloadErr)
		}
		if err := apply(manifest); err != nil {
			return err
		}
	}

	for _, doc := range kubernetesEngineCephCSIGeneratedManifests(cephCfg.CephPoolByClass, monitors, user, keyValue, strings.TrimSpace(mkeConf.CephFilesystemName)) {
		if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
			return err
		}
	}
	return nil
}

// kubernetesEngineCephCSIKeyFromKeyring は、ceph.client.admin.keyring形式のファイル内容から
// "key = <base64>" の値を取り出す。
func kubernetesEngineCephCSIKeyFromKeyring(content []byte) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if m := kubernetesEngineCephCSIKeyringKeyPattern.FindStringSubmatch(line); len(m) == 2 {
			return strings.TrimSpace(m[1]), nil
		}
	}
	return "", fmt.Errorf("key not found in keyring content")
}

// kubernetesEngineCephCSIGeneratedManifests は、ダウンロードした公式マニフェストでは
// 提供されない、クラスタ固有のConfigMap（モニターアドレス）、Secret（認証情報）、
// StorageClass（ceph_pool_by_classとの対応）を生成する。
func kubernetesEngineCephCSIGeneratedManifests(poolByClass map[string]string, monitors []string, user, key, filesystemName string) [][]byte {
	docs := [][]byte{
		[]byte(kubernetesEngineCephCSIConfigMapYAML(monitors)),
		[]byte(kubernetesEngineCephCSISecretYAML(kubernetesEngineCephCSIRBDSecretName, user, key)),
		[]byte(kubernetesEngineCephCSISecretYAML(kubernetesEngineCephCSICephFSSecretName, user, key)),
	}

	classes := make([]string, 0, len(poolByClass))
	for class := range poolByClass {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	for _, class := range classes {
		isDefault := class == kubernetesEngineCephCSIDefaultRBDStorageClass
		docs = append(docs, []byte(kubernetesEngineCephCSIRBDStorageClassYAML(class, poolByClass[class], isDefault)))
	}

	if filesystemName != "" {
		docs = append(docs, []byte(kubernetesEngineCephCSICephFSStorageClassYAML(filesystemName)))
	}
	return docs
}

func kubernetesEngineCephCSIConfigMapYAML(monitors []string) string {
	quoted := make([]string, 0, len(monitors))
	for _, m := range monitors {
		quoted = append(quoted, `"`+m+`"`)
	}
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: kube-system
data:
  config.json: |
    [
      {
        "clusterID": "%s",
        "monitors": [%s]
      }
    ]
`, kubernetesEngineCephCSIConfigMapName, kubernetesEngineCephCSIClusterID, strings.Join(quoted, ", "))
}

func kubernetesEngineCephCSISecretYAML(name, user, key string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: kube-system
stringData:
  userID: %s
  userKey: %s
`, name, user, key)
}

func kubernetesEngineCephCSIRBDStorageClassYAML(class, pool string, isDefault bool) string {
	annotations := ""
	if isDefault {
		annotations = "  annotations:\n    storageclass.kubernetes.io/is-default-class: \"true\"\n"
	}
	return fmt.Sprintf(`apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-rbd-%s
%sprovisioner: %s
parameters:
  clusterID: %s
  pool: %s
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: %s
  csi.storage.k8s.io/provisioner-secret-namespace: kube-system
  csi.storage.k8s.io/controller-expand-secret-name: %s
  csi.storage.k8s.io/controller-expand-secret-namespace: kube-system
  csi.storage.k8s.io/node-stage-secret-name: %s
  csi.storage.k8s.io/node-stage-secret-namespace: kube-system
reclaimPolicy: Delete
allowVolumeExpansion: true
`, class, annotations, kubernetesEngineCephCSIRBDProvisioner, kubernetesEngineCephCSIClusterID, pool,
		kubernetesEngineCephCSIRBDSecretName, kubernetesEngineCephCSIRBDSecretName, kubernetesEngineCephCSIRBDSecretName)
}

func kubernetesEngineCephCSICephFSStorageClassYAML(filesystemName string) string {
	return fmt.Sprintf(`apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-cephfs
provisioner: %s
parameters:
  clusterID: %s
  fsName: %s
  csi.storage.k8s.io/provisioner-secret-name: %s
  csi.storage.k8s.io/provisioner-secret-namespace: kube-system
  csi.storage.k8s.io/controller-expand-secret-name: %s
  csi.storage.k8s.io/controller-expand-secret-namespace: kube-system
  csi.storage.k8s.io/node-stage-secret-name: %s
  csi.storage.k8s.io/node-stage-secret-namespace: kube-system
reclaimPolicy: Delete
allowVolumeExpansion: true
`, kubernetesEngineCephCSICephFSProvisioner, kubernetesEngineCephCSIClusterID, filesystemName,
		kubernetesEngineCephCSICephFSSecretName, kubernetesEngineCephCSICephFSSecretName, kubernetesEngineCephCSICephFSSecretName)
}
