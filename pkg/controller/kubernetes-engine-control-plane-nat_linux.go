package controller

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// controlPlaneSysctlConfPath は ip_forward を永続化する marmot専用のsysctl設定ファイル。
const controlPlaneSysctlConfPath = "/etc/sysctl.d/99-marmot-mke.conf"

var (
	controlPlaneRunIPTables = func(args ...string) ([]byte, error) {
		return exec.Command("iptables", args...).CombinedOutput()
	}
	controlPlaneRunSysctl = func(args ...string) ([]byte, error) {
		return exec.Command("sysctl", args...).CombinedOutput()
	}
	controlPlaneWriteFile = func(path string, data []byte, perm os.FileMode) error {
		return os.WriteFile(path, data, perm)
	}
	controlPlaneEnableIPForward = enableControlPlaneIPForward
)

// controlPlaneIPTablesRule は冪等に追加・削除可能な単一のiptablesルールを表す。
type controlPlaneIPTablesRule struct {
	table string
	chain string
	spec  []string
}

func (r controlPlaneIPTablesRule) checkArgs() []string {
	return append([]string{"-t", r.table, "-C", r.chain}, r.spec...)
}

func (r controlPlaneIPTablesRule) addArgs() []string {
	return append([]string{"-t", r.table, "-A", r.chain}, r.spec...)
}

func (r controlPlaneIPTablesRule) deleteArgs() []string {
	return append([]string{"-t", r.table, "-D", r.chain}, r.spec...)
}

func kubernetesEngineControlPlaneDNATRule(hostBindAddress, internalIP string, port int) controlPlaneIPTablesRule {
	return controlPlaneIPTablesRule{
		table: "nat",
		chain: "PREROUTING",
		spec: []string{
			"-p", "tcp", "-d", hostBindAddress, "--dport", strconv.Itoa(port),
			"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", internalIP, port),
		},
	}
}

func kubernetesEngineControlPlaneMasqueradeRule(internalIP string, port int) controlPlaneIPTablesRule {
	return controlPlaneIPTablesRule{
		table: "nat",
		chain: "POSTROUTING",
		spec:  []string{"-p", "tcp", "-d", internalIP, "--dport", strconv.Itoa(port), "-j", "MASQUERADE"},
	}
}

func ensureControlPlaneIPTablesRule(rule controlPlaneIPTablesRule) error {
	if _, err := controlPlaneRunIPTables(rule.checkArgs()...); err == nil {
		return nil
	}
	if out, err := controlPlaneRunIPTables(rule.addArgs()...); err != nil {
		return fmt.Errorf("iptables -A %s failed: %w (output=%s)", rule.chain, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeControlPlaneIPTablesRule(rule controlPlaneIPTablesRule) error {
	if _, err := controlPlaneRunIPTables(rule.checkArgs()...); err != nil {
		// ルールが存在しない場合は削除不要。
		return nil
	}
	if out, err := controlPlaneRunIPTables(rule.deleteArgs()...); err != nil {
		return fmt.Errorf("iptables -D %s failed: %w (output=%s)", rule.chain, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func enableControlPlaneIPForward() error {
	if err := controlPlaneWriteFile(controlPlaneSysctlConfPath, []byte("net.ipv4.ip_forward=1\n"), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", controlPlaneSysctlConfPath, err)
	}
	if out, err := controlPlaneRunSysctl("-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("sysctl -w net.ipv4.ip_forward=1 failed: %w (output=%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// EnsureKubernetesEngineControlPlaneNAT は、hostBindAddress:port 宛の通信を
// コントロールプレーンnetns内のinternalIP:port へDNAT転送するための設定(冪等)を行う。
// hostVethが internalIP と同一サブネット内にIPを持つこと(SetupKubernetesEngineControlPlaneNetwork
// のHostCIDR設定)を前提に、ルートnetnsのL3フォワーディングでnetns境界を越えて配送される。
// MASQUERADEは、コントロールプレーンnetns側にデフォルトゲートウェイが無く戻りパケットが
// 発信元に正しく届かないため、戻りパケットの送信元をhostVethのIPに書き換えるために必要。
func EnsureKubernetesEngineControlPlaneNAT(hostBindAddress, internalIP string, port int) error {
	hostBindAddress = strings.TrimSpace(hostBindAddress)
	if hostBindAddress == "" {
		return fmt.Errorf("control plane host bind address is empty")
	}
	if net.ParseIP(hostBindAddress) == nil {
		return fmt.Errorf("invalid control plane host bind address %q", hostBindAddress)
	}
	if net.ParseIP(internalIP) == nil {
		return fmt.Errorf("invalid control plane internal IP address %q", internalIP)
	}
	if port <= 0 {
		return fmt.Errorf("invalid control plane API server port %d", port)
	}
	if err := controlPlaneEnableIPForward(); err != nil {
		return err
	}
	if err := ensureControlPlaneIPTablesRule(kubernetesEngineControlPlaneDNATRule(hostBindAddress, internalIP, port)); err != nil {
		return err
	}
	return ensureControlPlaneIPTablesRule(kubernetesEngineControlPlaneMasqueradeRule(internalIP, port))
}

// RemoveKubernetesEngineControlPlaneNAT は EnsureKubernetesEngineControlPlaneNAT で追加した
// DNAT/MASQUERADEルールを削除する(冪等、ルール不在時は何もしない)。
func RemoveKubernetesEngineControlPlaneNAT(hostBindAddress, internalIP string, port int) error {
	if err := removeControlPlaneIPTablesRule(kubernetesEngineControlPlaneMasqueradeRule(internalIP, port)); err != nil {
		return err
	}
	return removeControlPlaneIPTablesRule(kubernetesEngineControlPlaneDNATRule(hostBindAddress, internalIP, port))
}
