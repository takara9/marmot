package controller

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// DefaultEtcdSystemdUnitDir はクラスタ専用etcdのsystemdユニットファイルを配置するディレクトリ。
	DefaultEtcdSystemdUnitDir = "/etc/systemd/system"

	// DefaultEtcdDataDir はクラスタ専用etcdのデータディレクトリの親ディレクトリ。
	// 実際のデータは "<DefaultEtcdDataDir>/<cluster>" 配下に置かれる。
	DefaultEtcdDataDir = "/var/lib/marmot/mke/etcd-data"

	kubernetesEngineEtcdUnitPrefix = "mke-etcd-"
)

// KubernetesEngineEtcdUnitConfig はクラスタ専用etcdのsystemdユニット生成に必要なパラメータ。
type KubernetesEngineEtcdUnitConfig struct {
	ClusterName    string
	EtcdBinaryPath string
	DataDir        string
	ClientPort     int
	PeerPort       int
}

// systemctl呼び出しをテストから差し替え可能にするためのパッケージ変数群。
var (
	systemdDaemonReload = func() error { return exec.Command("systemctl", "daemon-reload").Run() }
	systemdEnableUnit   = func(unit string) error { return exec.Command("systemctl", "enable", unit).Run() }
	systemdDisableUnit  = func(unit string) error { return exec.Command("systemctl", "disable", unit).Run() }
	systemdStartUnit    = func(unit string) error { return exec.Command("systemctl", "start", unit).Run() }
	systemdStopUnit     = func(unit string) error { return exec.Command("systemctl", "stop", unit).Run() }
)

// etcdSystemdUnitDir はユニットファイルの配置先ディレクトリ。テストから上書き可能にするため
// パッケージ変数として保持する(既定値は DefaultEtcdSystemdUnitDir)。
var etcdSystemdUnitDir = DefaultEtcdSystemdUnitDir

// KubernetesEngineEtcdUnitName はクラスタ名から "mke-etcd-<cluster>.service" 形式の
// ユニット名を返す。
func KubernetesEngineEtcdUnitName(clusterName string) string {
	return kubernetesEngineEtcdUnitPrefix + strings.TrimSpace(clusterName) + ".service"
}

func kubernetesEngineEtcdUnitPath(clusterName string) string {
	return filepath.Join(etcdSystemdUnitDir, KubernetesEngineEtcdUnitName(clusterName))
}

// renderKubernetesEngineEtcdUnit はsystemdユニットファイルの内容を生成する。
func renderKubernetesEngineEtcdUnit(cfg KubernetesEngineEtcdUnitConfig) string {
	memberName := "mke-" + cfg.ClusterName
	return fmt.Sprintf(`[Unit]
Description=Marmot Kubernetes Engine dedicated etcd for cluster %s
After=network.target

[Service]
Type=notify
ExecStart=%s --name %s --data-dir %s --listen-client-urls http://127.0.0.1:%d --advertise-client-urls http://127.0.0.1:%d --listen-peer-urls http://127.0.0.1:%d --initial-advertise-peer-urls http://127.0.0.1:%d --initial-cluster %s=http://127.0.0.1:%d --initial-cluster-token mke-%s --initial-cluster-state new
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
`,
		cfg.ClusterName,
		cfg.EtcdBinaryPath, memberName, cfg.DataDir,
		cfg.ClientPort, cfg.ClientPort,
		cfg.PeerPort, cfg.PeerPort,
		memberName, cfg.PeerPort,
		cfg.ClusterName,
	)
}

// CreateKubernetesEngineEtcdUnit はクラスタ専用etcdのsystemdユニットファイルを生成・配置し、
// daemon-reload後に有効化・起動する。データディレクトリが存在しなければ作成する。
func CreateKubernetesEngineEtcdUnit(cfg KubernetesEngineEtcdUnitConfig) error {
	if strings.TrimSpace(cfg.ClusterName) == "" {
		return fmt.Errorf("cluster name is empty")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create etcd data dir: %w", err)
	}
	content := renderKubernetesEngineEtcdUnit(cfg)
	if err := os.WriteFile(kubernetesEngineEtcdUnitPath(cfg.ClusterName), []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write etcd unit file: %w", err)
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	unit := KubernetesEngineEtcdUnitName(cfg.ClusterName)
	if err := systemdEnableUnit(unit); err != nil {
		return fmt.Errorf("systemctl enable %s failed: %w", unit, err)
	}
	if err := systemdStartUnit(unit); err != nil {
		return fmt.Errorf("systemctl start %s failed: %w", unit, err)
	}
	return nil
}

// StopKubernetesEngineEtcdUnit はクラスタ専用etcdのユニットを停止する。
func StopKubernetesEngineEtcdUnit(clusterName string) error {
	unit := KubernetesEngineEtcdUnitName(clusterName)
	if err := systemdStopUnit(unit); err != nil {
		return fmt.Errorf("systemctl stop %s failed: %w", unit, err)
	}
	return nil
}

// DeleteKubernetesEngineEtcdUnit はクラスタ専用etcdのユニットを停止・無効化した上で
// ユニットファイルを削除し、daemon-reloadする。ユニットファイルが既に存在しない場合も
// エラーにはしない。
func DeleteKubernetesEngineEtcdUnit(clusterName string) error {
	name := strings.TrimSpace(clusterName)
	if name == "" {
		return fmt.Errorf("cluster name is empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("cluster name is invalid: %q", clusterName)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("cluster name contains invalid character %q", r)
	}

	unitPath := kubernetesEngineEtcdUnitPath(name)
	_, statErr := os.Stat(unitPath)
	unitFileMissing := os.IsNotExist(statErr)
	if statErr != nil && !unitFileMissing {
		return fmt.Errorf("failed to stat etcd unit file: %w", statErr)
	}

	unit := KubernetesEngineEtcdUnitName(name)
	if err := systemdStopUnit(unit); err != nil && !unitFileMissing {
		return fmt.Errorf("systemctl stop %s failed: %w", unit, err)
	}
	if err := systemdDisableUnit(unit); err != nil && !unitFileMissing {
		return fmt.Errorf("systemctl disable %s failed: %w", unit, err)
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove etcd unit file: %w", err)
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	return nil
}
