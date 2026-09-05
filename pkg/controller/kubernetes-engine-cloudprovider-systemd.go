package controller

import (
	"fmt"
	"os"
	"strings"
)

// KubernetesEngineCloudControllerManagerUnitConfig は cloud-controller-manager 用
// systemdユニット生成に必要なパラメータ。CCM本体はまだ実装されていないため、
// BinaryPath/ExtraArgsは将来の実バイナリに合わせて呼び出し側が指定する前提のスキャフォールドとする。
// CCMはkube-apiserver(ノード間通信用ネットワーク経由で到達可能)とmarmotd自身のREST API
// (ホストのルート名前空間でのみ待受け)の両方に接続する必要があるため、他のコントロールプレーン
// プロセスと異なりNetworkNamespacePathは使用せず、ルート名前空間で動作させる。
type KubernetesEngineCloudControllerManagerUnitConfig struct {
	ClusterName    string
	BinaryPath     string
	KubeconfigPath string
	ExtraArgs      []string
}

// KubernetesEngineCloudControllerManagerUnitName は
// "mke-cloud-controller-manager-<cluster>.service" 形式のユニット名を返す。
func KubernetesEngineCloudControllerManagerUnitName(clusterName string) string {
	return KubernetesEngineControlPlaneUnitName("cloud-controller-manager", clusterName)
}

func renderKubernetesEngineCloudControllerManagerUnit(cfg KubernetesEngineCloudControllerManagerUnitConfig) string {
	apiServerUnit := KubernetesEngineControlPlaneUnitName("kube-apiserver", cfg.ClusterName)
	// BinaryPath/フラグ体系は本物のk8s.io cloud-controller-managerではなく、mke-node-controller
	// (k8s.io/cloud-provider非依存の軽量reconciler)を前提とする。--kubeconfig以外のフラグは
	// 呼び出し側がExtraArgsで指定する。
	args := fmt.Sprintf("--kubeconfig=%s", cfg.KubeconfigPath)
	if len(cfg.ExtraArgs) > 0 {
		args = args + " " + strings.Join(cfg.ExtraArgs, " ")
	}
	return fmt.Sprintf(`[Unit]
Description=Marmot Kubernetes cloud-controller-manager for cluster %s
Requires=%s
After=%s

[Service]
Type=simple
ExecStart=%s %s
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
`, cfg.ClusterName, apiServerUnit, apiServerUnit, cfg.BinaryPath, args)
}

// CreateKubernetesEngineCloudControllerManagerUnit は cloud-controller-manager 用の
// systemdユニットを生成し、有効化・起動する。CCMはフェーズ14時点では任意導入のため、
// 既存のCreateKubernetesEngineControlPlaneUnitsとは独立して呼び出す。
func CreateKubernetesEngineCloudControllerManagerUnit(cfg KubernetesEngineCloudControllerManagerUnitConfig) error {
	name, err := validateKubernetesEngineEtcdClusterName(cfg.ClusterName)
	if err != nil {
		return err
	}
	cfg.ClusterName = name
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		return fmt.Errorf("cloud-controller-manager binary path is empty")
	}
	if strings.TrimSpace(cfg.KubeconfigPath) == "" {
		return fmt.Errorf("cloud-controller-manager kubeconfig path is empty")
	}

	unit := KubernetesEngineCloudControllerManagerUnitName(name)
	content := renderKubernetesEngineCloudControllerManagerUnit(cfg)
	if err := os.WriteFile(controlPlaneUnitPath("cloud-controller-manager", name), []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write cloud-controller-manager unit: %w", err)
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	if err := systemdEnableUnit(unit); err != nil {
		return fmt.Errorf("systemctl enable %s failed: %w", unit, err)
	}
	if err := systemdStartUnit(unit); err != nil {
		_ = systemdDisableUnit(unit)
		return fmt.Errorf("systemctl start %s failed: %w", unit, err)
	}
	return nil
}

// DeleteKubernetesEngineCloudControllerManagerUnit は cloud-controller-manager 用ユニットを
// 停止・無効化・削除する。CCMが未導入(ユニット不在)の場合は何もせず成功として扱う。
func DeleteKubernetesEngineCloudControllerManagerUnit(clusterName string) error {
	name, err := validateKubernetesEngineEtcdClusterName(clusterName)
	if err != nil {
		return err
	}
	unit := KubernetesEngineCloudControllerManagerUnitName(name)
	if err := systemdStopUnit(unit); err != nil && !isSystemdUnitMissingError(err) {
		return fmt.Errorf("systemctl stop %s failed: %w", unit, err)
	}
	if err := systemdDisableUnit(unit); err != nil && !isSystemdUnitMissingError(err) {
		return fmt.Errorf("systemctl disable %s failed: %w", unit, err)
	}
	if err := os.Remove(controlPlaneUnitPath("cloud-controller-manager", name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cloud-controller-manager unit: %w", err)
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	return nil
}
