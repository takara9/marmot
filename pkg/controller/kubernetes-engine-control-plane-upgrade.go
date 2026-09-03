package controller

import (
	"fmt"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

// kubernetesEngineMajorMinor は "1.36"、"v1.36"、"v1.36.2" のようなバージョン文字列から
// major/minorのみを取り出す(patchの違いはspec.version変更とはみなさない)。
func kubernetesEngineMajorMinor(version string) (int, int, error) {
	matches := kubernetesVersionPattern.FindStringSubmatch(version)
	if matches == nil {
		return 0, 0, fmt.Errorf("invalid Kubernetes version %q", version)
	}
	var major, minor int
	if _, err := fmt.Sscanf(matches[1], "%d", &major); err != nil {
		return 0, 0, fmt.Errorf("invalid Kubernetes version %q", version)
	}
	if _, err := fmt.Sscanf(matches[2], "%d", &minor); err != nil {
		return 0, 0, fmt.Errorf("invalid Kubernetes version %q", version)
	}
	return major, minor, nil
}

// kubernetesEngineNeedsUpgrade は、spec.versionとコントロールプレーンで解決済みの
// Kubernetesバージョン(major.minor)を比較し、アップグレードが必要かどうかを返す。
// ダウングレードはUpdateKubernetesEngineSpec側で既に拒否されているため、ここでは
// 単純な不一致判定のみを行う。
func kubernetesEngineNeedsUpgrade(ke api.KubernetesEngine) (bool, error) {
	if ke.Status == nil || ke.Status.ResolvedKubernetesVersion == nil {
		return false, fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	specMajor, specMinor, err := kubernetesEngineMajorMinor(ke.Spec.Version)
	if err != nil {
		return false, err
	}
	resolvedMajor, resolvedMinor, err := kubernetesEngineMajorMinor(*ke.Status.ResolvedKubernetesVersion)
	if err != nil {
		return false, err
	}
	return specMajor != resolvedMajor || specMinor != resolvedMinor, nil
}

// UpgradeKubernetesEngineControlPlane は、spec.versionに合わせてコントロールプレーン
// (kube-apiserver/kube-scheduler/kube-controller-manager)のバイナリを再取得し、
// systemdユニットを再生成したうえで再起動する。あわせて専用etcdの再確認も行う
// (etcdバージョンはmkeConf側の固定値であり、spec.versionには連動しないが、
// メンテナンスウィンドウ内での差し替えを許容するため同じタイミングで再実行する)。
// 再起動後にヘルスチェックが通った時点でStatus.ResolvedKubernetesVersionを更新する。
func UpgradeKubernetesEngineControlPlane(database *db.Database, mkeConf *marmotd.MKEConfig, ownEtcdURL string, ke api.KubernetesEngine) error {
	clusterName, err := validateKubernetesEngineEtcdClusterName(ke.Metadata.Name)
	if err != nil {
		return err
	}
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ControlPlaneHostIpAddress == nil ||
		ke.Status.ApiServerPort == nil || ke.Status.EtcdClientPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}

	resolvedVersion, binaries, err := EnsureKubernetesControlPlaneBinaries(DefaultKubernetesControlPlaneBinaryDir, ke.Spec.Version)
	if err != nil {
		return fmt.Errorf("failed to resolve/download control plane binaries for %s: %w", ke.Spec.Version, err)
	}

	if _, _, err := ProvisionKubernetesEngineEtcd(database, mkeConf, ownEtcdURL, ke); err != nil {
		return fmt.Errorf("failed to refresh etcd during control plane upgrade: %w", err)
	}

	assets, err := EnsureKubernetesEngineControlPlaneAssets(DefaultKubernetesEnginePkiDir, DefaultKubernetesControlPlaneConfigDir, clusterName,
		*ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort, mkeConf.ControlPlaneBindAddress)
	if err != nil {
		return err
	}
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return err
	}
	unitCfg := KubernetesEngineControlPlaneUnitConfig{
		ClusterName:        clusterName,
		NetworkNamespace:   namespace,
		APIServerIP:        *ke.Status.ControlPlaneIpAddress,
		APIServerPort:      *ke.Status.ApiServerPort,
		EtcdClientPort:     *ke.Status.EtcdClientPort,
		Binaries:           binaries,
		Assets:             assets,
		ServiceClusterCIDR: DefaultKubernetesServiceClusterCIDR,
	}
	if err := CreateKubernetesEngineControlPlaneUnits(unitCfg); err != nil {
		return fmt.Errorf("failed to write upgraded control plane units: %w", err)
	}
	// CreateKubernetesEngineControlPlaneUnitsのsystemctl startは既にactiveなユニットには何もしないため、
	// 新バイナリを反映させるために明示的に再起動する(kube-apiserver→scheduler→controller-managerの順)。
	for _, component := range kubernetesControlPlaneBinaries {
		unit := KubernetesEngineControlPlaneUnitName(component, clusterName)
		if err := systemdRestartUnit(unit); err != nil {
			return fmt.Errorf("failed to restart %s: %w", unit, err)
		}
	}
	if err := CheckKubernetesEngineControlPlaneHealth(namespace, assets.CACertPath, *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort); err != nil {
		return fmt.Errorf("control plane health check failed after upgrade: %w", err)
	}
	return database.UpdateKubernetesEngineControlPlaneStatus(api.KubernetesEngineID(ke),
		*ke.Status.ControlPlaneIpAddress, *ke.Status.ControlPlaneHostIpAddress, *ke.Status.ApiServerPort, resolvedVersion)
}
