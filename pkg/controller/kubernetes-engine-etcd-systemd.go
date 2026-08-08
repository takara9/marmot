package controller

import (
	"errors"
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
	ClusterName      string
	EtcdBinaryPath   string
	DataDir          string
	NetworkNamespace string
	ClientPort       int
	PeerPort         int
}

// systemctl呼び出しをテストから差し替え可能にするためのパッケージ変数群。
// stop/disableは、ユニット不在による失敗かどうかを呼び出し元が判別できるよう
// systemdUnitMissingError でラップする(runSystemctlUnitCommand参照)。
var (
	systemdDaemonReload = func() error { return exec.Command("systemctl", "daemon-reload").Run() }
	systemdEnableUnit   = func(unit string) error { return exec.Command("systemctl", "enable", unit).Run() }
	systemdDisableUnit  = func(unit string) error { return runSystemctlUnitCommand("disable", unit) }
	systemdStartUnit    = func(unit string) error { return exec.Command("systemctl", "start", unit).Run() }
	systemdStopUnit     = func(unit string) error { return runSystemctlUnitCommand("stop", unit) }
)

// systemdUnitMissingError はsystemctlがユニット不在を理由に失敗したことを表す。
type systemdUnitMissingError struct{ err error }

func (e *systemdUnitMissingError) Error() string { return e.err.Error() }
func (e *systemdUnitMissingError) Unwrap() error { return e.err }

// isSystemdUnitMissingError はerrがユニット不在に起因するものかを判定する。
func isSystemdUnitMissingError(err error) bool {
	var missing *systemdUnitMissingError
	return errors.As(err, &missing)
}

// unitNotFoundOutputPatterns はsystemctlの出力に含まれる「ユニット不在」を示す
// 代表的な文字列。systemdのバージョンによって文言が異なるため複数パターンを見る。
var unitNotFoundOutputPatterns = []string{
	"not loaded",
	"not-found",
	"no such file or directory",
	"does not exist",
	"no such unit",
}

// runSystemctlUnitCommand は `systemctl <action> <unit>` を実行し、失敗時に出力内容から
// ユニット不在による失敗かどうかを判定してエラーをラップする。
func runSystemctlUnitCommand(action, unit string) error {
	out, err := exec.Command("systemctl", action, unit).CombinedOutput()
	if err == nil {
		return nil
	}
	wrapped := fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	lower := strings.ToLower(string(out))
	for _, pattern := range unitNotFoundOutputPatterns {
		if strings.Contains(lower, pattern) {
			return &systemdUnitMissingError{err: wrapped}
		}
	}
	return wrapped
}

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

// validateKubernetesEngineEtcdClusterName はクラスタ名がユニット名/パスの構成要素として
// 安全であることを検証し、前後の空白を除いた名前を返す。
func validateKubernetesEngineEtcdClusterName(clusterName string) (string, error) {
	name := strings.TrimSpace(clusterName)
	if name == "" {
		return "", fmt.Errorf("cluster name is empty")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("cluster name is invalid: %q", clusterName)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", fmt.Errorf("cluster name contains invalid character %q", r)
	}
	return name, nil
}

// renderKubernetesEngineEtcdUnit はsystemdユニットファイルの内容を生成する。
func renderKubernetesEngineEtcdUnit(cfg KubernetesEngineEtcdUnitConfig) string {
	memberName := "mke-" + cfg.ClusterName
	ns := strings.TrimSpace(cfg.NetworkNamespace)
	networkNamespace := ""
	if ns != "" && !strings.ContainsAny(ns, "/\n\r\t ") {
		networkNamespace = fmt.Sprintf("NetworkNamespacePath=/run/netns/%s\n", ns)
	}
	return fmt.Sprintf(`[Unit]
Description=Marmot Kubernetes Engine dedicated etcd for cluster %s
After=network.target

[Service]
Type=notify
%sExecStart=%s --name %s --data-dir %s --listen-client-urls http://127.0.0.1:%d --advertise-client-urls http://127.0.0.1:%d --listen-peer-urls http://127.0.0.1:%d --initial-advertise-peer-urls http://127.0.0.1:%d --initial-cluster %s=http://127.0.0.1:%d --initial-cluster-token mke-%s --initial-cluster-state new
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
`,
		cfg.ClusterName,
		networkNamespace,
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
	name, err := validateKubernetesEngineEtcdClusterName(cfg.ClusterName)
	if err != nil {
		return err
	}
	cfg.ClusterName = name
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
	name, err := validateKubernetesEngineEtcdClusterName(clusterName)
	if err != nil {
		return err
	}
	unit := KubernetesEngineEtcdUnitName(name)
	if err := systemdStopUnit(unit); err != nil {
		return fmt.Errorf("systemctl stop %s failed: %w", unit, err)
	}
	return nil
}

// DeleteKubernetesEngineEtcdUnit はクラスタ専用etcdのユニットを停止・無効化した上で
// ユニットファイルを削除し、daemon-reloadする。ユニット不在に起因するstop/disableの
// 失敗は無視するが、それ以外の失敗(権限不足等)は冪等性の対象外としてエラーを返す。
func DeleteKubernetesEngineEtcdUnit(clusterName string) error {
	name, err := validateKubernetesEngineEtcdClusterName(clusterName)
	if err != nil {
		return err
	}

	unit := KubernetesEngineEtcdUnitName(name)
	if err := systemdStopUnit(unit); err != nil && !isSystemdUnitMissingError(err) {
		return fmt.Errorf("systemctl stop %s failed: %w", unit, err)
	}
	if err := systemdDisableUnit(unit); err != nil && !isSystemdUnitMissingError(err) {
		return fmt.Errorf("systemctl disable %s failed: %w", unit, err)
	}
	if err := os.Remove(kubernetesEngineEtcdUnitPath(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove etcd unit file: %w", err)
	}
	if err := systemdDaemonReload(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}
	return nil
}
