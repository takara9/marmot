package controller

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

// DefaultKubernetesEngineMKEManifestsDir は、パッケージインストール時に mke/ 配下の
// CSIインストールマニフェストがコピーされるベースディレクトリ。
// クラスタ作成時に、この配下がクラスタ専用ディレクトリ(<DefaultKubernetesEngineMKEManifestsDir>/<cluster>)
// へさらに複製され、クラスタ固有の値(clusterID/monitors/認証情報)が書き込まれる。
const DefaultKubernetesEngineMKEManifestsDir = "/var/lib/marmot/mke-manifests"

// kubernetesEngineCephCSITemplateEntries は、ベースディレクトリからクラスタ専用ディレクトリへ
// コピーする対象(ファイル・ディレクトリ)の一覧。
var kubernetesEngineCephCSITemplateEntries = []string{
	"ceph-conf.yaml",
	"csi-config-map.yaml",
	"ceph-rbd",
	"ceph-fs",
	"kms",
}

// kubernetesEngineCephCSIApplyOrder は、Ceph CSI連携マニフェストを適用する順序
// (クラスタ専用ディレクトリからの相対パス)。RBACやKMSの前提リソースを、それらを
// 参照するDeployment/Secretより先に適用する必要があるため、順序は固定とする。
var kubernetesEngineCephCSIApplyOrder = []string{
	"ceph-conf.yaml",
	"csi-config-map.yaml",
	filepath.Join("ceph-rbd", "csi-provisioner-rbac.yaml"),
	filepath.Join("ceph-rbd", "csi-nodeplugin-rbac.yaml"),
	filepath.Join("ceph-rbd", "csi-rbdplugin-provisioner.yaml"),
	filepath.Join("ceph-rbd", "csi-rbdplugin.yaml"),
	filepath.Join("ceph-rbd", "csidriver.yaml"),
	filepath.Join("kms", "vault.yaml"),
	filepath.Join("kms", "csi-vaulttokenreview-rbac.yaml"),
	filepath.Join("kms", "kms-config.yaml"),
	filepath.Join("ceph-rbd", "csi-rbd-secret.yaml"),
	filepath.Join("ceph-rbd", "rbd-storageclass.yaml"),
	filepath.Join("ceph-fs", "csi-provisioner-rbac.yaml"),
	filepath.Join("ceph-fs", "csi-nodeplugin-rbac.yaml"),
	filepath.Join("ceph-fs", "csi-cephfsplugin-provisioner.yaml"),
	filepath.Join("ceph-fs", "csi-cephfsplugin.yaml"),
	filepath.Join("ceph-fs", "csidriver.yaml"),
	filepath.Join("ceph-fs", "csi-cephfs-secret.yaml"),
	filepath.Join("ceph-fs", "cephfs-storageclass.yaml"),
}

// kubernetesEngineCephCSIValues は、mke-manifestsテンプレートへ書き込むクラスタ固有の値。
type kubernetesEngineCephCSIValues struct {
	ClusterID     string
	Monitors      []string
	RBDUserID     string
	RBDUserKey    string
	CephFSUserID  string
	CephFSUserKey string
}

// EnsureKubernetesEngineCephCSI は、marmotd.json の ceph_enabled=true の場合に、mke.json の
// Ceph接続情報を /var/lib/marmot/mke-manifests/<cluster> 配下のマニフェストへ反映したうえで、
// Ceph CSI(RBD/CephFS、KMS含む)一式をコントロールプレーンのAPIサーバーへ適用する。
// Cephが無効な場合は何もしない(no-op)。
//
// 制約: マニフェストの適用は「存在しなければ作成する」のみを行い、既存リソースの更新は
// 行わない。mke.json の接続情報を変更した場合、反映するには /var/lib/marmot/mke-manifests/<cluster>
// を削除し、かつ既存のConfigMap/Secret/StorageClassをクラスタから削除してから再実行すること。
func EnsureKubernetesEngineCephCSI(mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) error {
	cephCfg := marmotd.CurrentConfig()
	if !cephCfg.CephEnabled {
		return nil
	}
	values, err := kubernetesEngineCephCSIValuesFromConfig(mkeConf)
	if err != nil {
		return err
	}
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}

	clusterName := strings.TrimSpace(ke.Metadata.Name)
	clusterDir, err := kubernetesEngineCephCSIClusterManifestsDir(DefaultKubernetesEngineMKEManifestsDir, clusterName)
	if err != nil {
		return err
	}
	if err := prepareKubernetesEngineCephCSIManifests(DefaultKubernetesEngineMKEManifestsDir, clusterDir, values); err != nil {
		return err
	}

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

	for _, rel := range kubernetesEngineCephCSIApplyOrder {
		content, err := os.ReadFile(filepath.Join(clusterDir, rel))
		if err != nil {
			return fmt.Errorf("failed to read Ceph CSI manifest %s: %w", rel, err)
		}
		for _, doc := range splitKubernetesEngineYAMLDocuments(content) {
			if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, doc); err != nil {
				return fmt.Errorf("failed to apply Ceph CSI manifest %s: %w", rel, err)
			}
		}
	}
	return nil
}

// RemoveKubernetesEngineCephCSIManifests は、クラスタ削除時に呼び出し、
// /var/lib/marmot/mke-manifests/<cluster> を削除する。クラスタ専用ディレクトリが
// 存在しない場合(Cephが無効なクラスタ等)は何もしない。
func RemoveKubernetesEngineCephCSIManifests(clusterName string) error {
	return removeKubernetesEngineCephCSIManifests(DefaultKubernetesEngineMKEManifestsDir, clusterName)
}

func removeKubernetesEngineCephCSIManifests(baseDir, clusterName string) error {
	clusterDir, err := kubernetesEngineCephCSIClusterManifestsDir(baseDir, clusterName)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(clusterDir); err != nil {
		return fmt.Errorf("failed to remove Ceph CSI manifests dir: %w", err)
	}
	return nil
}

func kubernetesEngineCephCSIValuesFromConfig(mkeConf *marmotd.MKEConfig) (kubernetesEngineCephCSIValues, error) {
	clusterID := strings.TrimSpace(mkeConf.CephClusterID)
	if clusterID == "" {
		return kubernetesEngineCephCSIValues{}, fmt.Errorf("ceph_enabled=true requires clusterID to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	if len(mkeConf.CephMonitors) == 0 {
		return kubernetesEngineCephCSIValues{}, fmt.Errorf("ceph_enabled=true requires monitors to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	rbdUserID := strings.TrimSpace(mkeConf.CephRBDUserId)
	rbdUserKey := strings.TrimSpace(mkeConf.CephRBDUserKey)
	if rbdUserID == "" || rbdUserKey == "" {
		return kubernetesEngineCephCSIValues{}, fmt.Errorf("ceph_enabled=true requires rbdUserId/rbdUserKey to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	cephfsUserID := strings.TrimSpace(mkeConf.CephFSUserId)
	cephfsUserKey := strings.TrimSpace(mkeConf.CephFSUserKey)
	if cephfsUserID == "" || cephfsUserKey == "" {
		return kubernetesEngineCephCSIValues{}, fmt.Errorf("ceph_enabled=true requires cephfsUserId/cephfsUserKey to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	return kubernetesEngineCephCSIValues{
		ClusterID:     clusterID,
		Monitors:      mkeConf.CephMonitors,
		RBDUserID:     rbdUserID,
		RBDUserKey:    rbdUserKey,
		CephFSUserID:  cephfsUserID,
		CephFSUserKey: cephfsUserKey,
	}, nil
}

func kubernetesEngineCephCSIClusterManifestsDir(baseDir, clusterName string) (string, error) {
	name, err := validateKubernetesEnginePkiClusterName(clusterName)
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, name), nil
}

// prepareKubernetesEngineCephCSIManifests は、baseDir配下のテンプレートをclusterDirへコピーし
// (既にコピー済みの場合は何もしない=冪等)、クラスタ固有の値をテンプレートへ書き込む。
func prepareKubernetesEngineCephCSIManifests(baseDir, clusterDir string, values kubernetesEngineCephCSIValues) error {
	if _, err := os.Stat(clusterDir); err == nil {
		// If a previous run failed after creating the directory, don't silently no-op.
		if _, err := os.Stat(filepath.Join(clusterDir, "csi-config-map.yaml")); err != nil {
			return fmt.Errorf("Ceph CSI manifests cluster dir exists but looks incomplete; remove %s and retry: %w", clusterDir, err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat Ceph CSI manifests cluster dir: %w", err)
	}

	// Ceph認証情報を含むため、クラスタ専用ディレクトリはroot以外から読めないようにする。
	if err := os.MkdirAll(clusterDir, 0o700); err != nil {
		return fmt.Errorf("failed to create Ceph CSI manifests cluster dir: %w", err)
	}
	for _, entry := range kubernetesEngineCephCSITemplateEntries {
		if err := copyKubernetesEngineCephCSIEntry(filepath.Join(baseDir, entry), filepath.Join(clusterDir, entry)); err != nil {
			return fmt.Errorf("failed to copy Ceph CSI manifest template %s: %w", entry, err)
		}
	}

	type templateEdit struct {
		rel        string
		secretFile bool
		edit       func(string) (string, error)
	}
	edits := []templateEdit{
		{rel: "csi-config-map.yaml", edit: func(content string) (string, error) {
			return kubernetesEngineRenderCephCSIConfigMap(content, values.ClusterID, values.Monitors), nil
		}},
		{rel: filepath.Join("ceph-rbd", "csi-rbd-secret.yaml"), secretFile: true, edit: func(content string) (string, error) {
			return kubernetesEngineSetYAMLSecretValues(content, values.RBDUserID, values.RBDUserKey)
		}},
		{rel: filepath.Join("ceph-rbd", "rbd-storageclass.yaml"), edit: func(content string) (string, error) {
			return kubernetesEngineSetYAMLScalar(content, "clusterID", values.ClusterID)
		}},
		{rel: filepath.Join("ceph-fs", "csi-cephfs-secret.yaml"), secretFile: true, edit: func(content string) (string, error) {
			return kubernetesEngineSetYAMLSecretValues(content, values.CephFSUserID, values.CephFSUserKey)
		}},
		{rel: filepath.Join("ceph-fs", "cephfs-storageclass.yaml"), edit: func(content string) (string, error) {
			return kubernetesEngineSetYAMLScalar(content, "clusterID", values.ClusterID)
		}},
	}
	for _, e := range edits {
		path := filepath.Join(clusterDir, e.rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", e.rel, err)
		}
		updated, err := e.edit(string(content))
		if err != nil {
			return fmt.Errorf("failed to render %s: %w", e.rel, err)
		}
		mode := os.FileMode(0o644)
		if e.secretFile {
			// Ceph認証情報の平文を含むため、他ユーザーから読めないようにする。
			mode = 0o600
		}
		if err := os.WriteFile(path, []byte(updated), mode); err != nil {
			return fmt.Errorf("failed to write %s: %w", e.rel, err)
		}
	}
	return nil
}

// copyKubernetesEngineCephCSIEntry は、ファイルまたはディレクトリ(再帰的に)をコピーする。
func copyKubernetesEngineCephCSIEntry(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyKubernetesEngineCephCSIFile(src, dst)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyKubernetesEngineCephCSIFile(path, target)
	})
}

func copyKubernetesEngineCephCSIFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

var (
	kubernetesEngineCephCSIConfigMapClusterIDPattern = regexp.MustCompile(`"clusterID":\s*"[^"]*"`)
	kubernetesEngineCephCSIConfigMapMonitorsPattern  = regexp.MustCompile(`(?s)"monitors":\s*\[.*?\]`)
)

// kubernetesEngineRenderCephCSIConfigMap は、csi-config-map.yaml に埋め込まれたJSON
// (data.config.json)内の "clusterID" / "monitors" の値を、クラスタ固有の値へ書き換える。
func kubernetesEngineRenderCephCSIConfigMap(content, clusterID string, monitors []string) string {
	content = kubernetesEngineCephCSIConfigMapClusterIDPattern.ReplaceAllStringFunc(content, func(string) string {
		return `"clusterID": ` + strconv.Quote(clusterID)
	})
	quoted := make([]string, len(monitors))
	for i, m := range monitors {
		quoted[i] = strconv.Quote(m)
	}
	replacement := `"monitors": [` + strings.Join(quoted, ", ") + `]`
	content = kubernetesEngineCephCSIConfigMapMonitorsPattern.ReplaceAllStringFunc(content, func(string) string {
		return replacement
	})
	return content
}

// kubernetesEngineSetYAMLScalar は、"key: value" 形式のYAMLマッピングエントリの値を書き換える。
// keyが見つからない場合はエラーを返す。
func kubernetesEngineSetYAMLScalar(content, key, value string) (string, error) {
	pattern := regexp.MustCompile(`(?m)^(\s*` + regexp.QuoteMeta(key) + `:)\s*.*$`)
	if !pattern.MatchString(content) {
		return "", fmt.Errorf("key %q not found in manifest", key)
	}
	return pattern.ReplaceAllStringFunc(content, func(line string) string {
		prefix := pattern.FindStringSubmatch(line)[1]
		return prefix + " " + value
	}), nil
}

// kubernetesEngineSetYAMLSecretValues は、Secretマニフェストの userID/userKey を書き換える。
func kubernetesEngineSetYAMLSecretValues(content, userID, userKey string) (string, error) {
	content, err := kubernetesEngineSetYAMLScalar(content, "userID", userID)
	if err != nil {
		return "", err
	}
	return kubernetesEngineSetYAMLScalar(content, "userKey", userKey)
}
