package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultKubernetesAPIServerPort = 6443
	controlPlaneHealthTimeout      = 30 * time.Second
)

var controlPlaneSystemdUnitDir = DefaultEtcdSystemdUnitDir
var controlPlaneHealthCommand = func(ctx context.Context, namespace, caPath, endpoint string) error {
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl", "--fail", "--silent", "--show-error", "--cacert", caPath, endpoint).CombinedOutput()
	if err != nil {
		return fmt.Errorf("control plane health check failed: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

type KubernetesEngineControlPlaneUnitConfig struct {
	ClusterName        string
	NetworkNamespace   string
	APIServerIP        string
	APIServerPort      int
	EtcdClientPort     int
	Binaries           map[string]string
	Assets             KubernetesEngineControlPlaneAssets
	ServiceClusterCIDR string
}

func KubernetesEngineControlPlaneUnitName(component, clusterName string) string {
	return "mke-" + component + "-" + strings.TrimSpace(clusterName) + ".service"
}

func controlPlaneUnitPath(component, clusterName string) string {
	return filepath.Join(controlPlaneSystemdUnitDir, KubernetesEngineControlPlaneUnitName(component, clusterName))
}

func renderKubernetesEngineControlPlaneUnits(cfg KubernetesEngineControlPlaneUnitConfig) map[string]string {
	apiServerUnit := KubernetesEngineControlPlaneUnitName("kube-apiserver", cfg.ClusterName)
	etcdUnit := KubernetesEngineEtcdUnitName(cfg.ClusterName)
	// --kubelet-preferred-address-types=InternalIP: コントロールプレーンnetnsからはホストのDNSに
	// 到達できないため、kubectl exec/logs等でノードのHostnameアドレスをDNS解決させず、
	// 到達可能なInternalIP(node-ip)へ直接接続させる。
	apiServer := fmt.Sprintf(`[Unit]
Description=Marmot Kubernetes API server for cluster %s
Requires=%s
After=%s

[Service]
Type=simple
NetworkNamespacePath=/run/netns/%s
ExecStart=%s --advertise-address=%s --bind-address=0.0.0.0 --secure-port=%d --etcd-servers=http://127.0.0.1:%d --client-ca-file=%s --tls-cert-file=%s --tls-private-key-file=%s --service-account-key-file=%s --service-account-signing-key-file=%s --service-account-issuer=https://kubernetes.default.svc.cluster.local --service-cluster-ip-range=%s --authorization-mode=Node,RBAC --allow-privileged=true --kubelet-preferred-address-types=InternalIP --kubelet-client-certificate=%s --kubelet-client-key=%s
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
`, cfg.ClusterName, etcdUnit, etcdUnit, cfg.NetworkNamespace, cfg.Binaries["kube-apiserver"], cfg.APIServerIP, cfg.APIServerPort, cfg.EtcdClientPort, cfg.Assets.CACertPath, cfg.Assets.APIServerCertPath, cfg.Assets.APIServerKeyPath, cfg.Assets.ServiceAccountPublicKeyPath, cfg.Assets.ServiceAccountPrivateKeyPath, cfg.ServiceClusterCIDR, cfg.Assets.KubeletClientCertPath, cfg.Assets.KubeletClientKeyPath)

	scheduler := fmt.Sprintf(`[Unit]
Description=Marmot Kubernetes scheduler for cluster %s
Requires=%s
After=%s

[Service]
Type=simple
NetworkNamespacePath=/run/netns/%s
ExecStart=%s --kubeconfig=%s
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
`, cfg.ClusterName, apiServerUnit, apiServerUnit, cfg.NetworkNamespace, cfg.Binaries["kube-scheduler"], cfg.Assets.SchedulerKubeconfigPath)

	controllerManager := fmt.Sprintf(`[Unit]
Description=Marmot Kubernetes controller manager for cluster %s
Requires=%s
After=%s

[Service]
Type=simple
NetworkNamespacePath=/run/netns/%s
ExecStart=%s --kubeconfig=%s --root-ca-file=%s --service-account-private-key-file=%s --use-service-account-credentials=true
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
`, cfg.ClusterName, apiServerUnit, apiServerUnit, cfg.NetworkNamespace, cfg.Binaries["kube-controller-manager"], cfg.Assets.ControllerManagerConfigPath, cfg.Assets.CACertPath, cfg.Assets.ServiceAccountPrivateKeyPath)

	return map[string]string{
		"kube-apiserver":          apiServer,
		"kube-scheduler":          scheduler,
		"kube-controller-manager": controllerManager,
	}
}

func CreateKubernetesEngineControlPlaneUnits(cfg KubernetesEngineControlPlaneUnitConfig) error {
	name, err := validateKubernetesEngineEtcdClusterName(cfg.ClusterName)
	if err != nil {
		return err
	}
	cfg.ClusterName = name
	cfg.NetworkNamespace = strings.TrimSpace(cfg.NetworkNamespace)
	if cfg.NetworkNamespace == "" || strings.ContainsAny(cfg.NetworkNamespace, "/\n\r\t ") {
		return fmt.Errorf("invalid network namespace %q", cfg.NetworkNamespace)
	}
	if cfg.APIServerPort <= 0 || cfg.EtcdClientPort <= 0 {
		return fmt.Errorf("API server and etcd ports must be positive")
	}
	for _, component := range kubernetesControlPlaneBinaries {
		if strings.TrimSpace(cfg.Binaries[component]) == "" {
			return fmt.Errorf("binary path for %s is empty", component)
		}
	}
	units := renderKubernetesEngineControlPlaneUnits(cfg)
	for _, component := range kubernetesControlPlaneBinaries {
		if err := os.WriteFile(controlPlaneUnitPath(component, name), []byte(units[component]), 0o644); err != nil {
			return fmt.Errorf("failed to write %s unit: %w", component, err)
		}
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	started := make([]string, 0, len(kubernetesControlPlaneBinaries))
	for _, component := range kubernetesControlPlaneBinaries {
		unit := KubernetesEngineControlPlaneUnitName(component, name)
		if err := systemdEnableUnit(unit); err != nil {
			rollbackControlPlaneUnits(started)
			return fmt.Errorf("systemctl enable %s failed: %w", unit, err)
		}
		if err := systemdStartUnit(unit); err != nil {
			_ = systemdDisableUnit(unit)
			rollbackControlPlaneUnits(started)
			return fmt.Errorf("systemctl start %s failed: %w", unit, err)
		}
		started = append(started, unit)
	}
	return nil
}

func DeleteKubernetesEngineControlPlaneUnits(clusterName string) error {
	name, err := validateKubernetesEngineEtcdClusterName(clusterName)
	if err != nil {
		return err
	}
	for index := len(kubernetesControlPlaneBinaries) - 1; index >= 0; index-- {
		component := kubernetesControlPlaneBinaries[index]
		unit := KubernetesEngineControlPlaneUnitName(component, name)
		if err := systemdStopUnit(unit); err != nil && !isSystemdUnitMissingError(err) {
			return fmt.Errorf("systemctl stop %s failed: %w", unit, err)
		}
		if err := systemdDisableUnit(unit); err != nil && !isSystemdUnitMissingError(err) {
			return fmt.Errorf("systemctl disable %s failed: %w", unit, err)
		}
		if err := os.Remove(controlPlaneUnitPath(component, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s unit: %w", component, err)
		}
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	return nil
}

func CheckKubernetesEngineControlPlaneHealth(namespace, caPath, apiServerIP string, apiServerPort int) error {
	ctx, cancel := context.WithTimeout(context.Background(), controlPlaneHealthTimeout)
	defer cancel()
	endpoint := fmt.Sprintf("https://%s:%d/healthz", apiServerIP, apiServerPort)
	var lastErr error
	for {
		if err := controlPlaneHealthCommand(ctx, namespace, caPath, endpoint); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("control plane did not become healthy: %w", lastErr)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func rollbackControlPlaneUnits(started []string) {
	for index := len(started) - 1; index >= 0; index-- {
		_ = systemdStopUnit(started[index])
		_ = systemdDisableUnit(started[index])
	}
}
