package marmotd

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

const (
	testMockHostIDOutput     = "0x00000001\n"
	iscsiInitiatornamePrefix = "iqn.2004-10.com.marmot:"
)

var hostIDCommandOutput = func() ([]byte, error) {
	return exec.Command("hostid").Output()
}

var iscsiInitiatornameFile = "/etc/iscsi/initiatorname.iscsi"

var systemctlRestartIscsid = func() error {
	return exec.Command("systemctl", "restart", "iscsid").Run()
}

func runningInTestBinary() bool {
	return strings.HasSuffix(filepath.Base(os.Args[0]), ".test")
}

func init() {
	// テストバイナリ実行時は hostid コマンド依存を避けるためモックを利用する。
	if runningInTestBinary() {
		hostIDCommandOutput = func() ([]byte, error) {
			return []byte(testMockHostIDOutput), nil
		}
		systemctlRestartIscsid = func() error {
			return nil
		}
	}
}

// ホストの状態情報を収集してetcdに保存する
func (m *Marmot) CollectAndUpdateHostStatus() error {
	status, err := m.CollectHostStatus()
	if err != nil {
		slog.Error("CollectHostStatus()", "err", err)
		return err
	}
	if err := m.Db.PutHostStatus(status); err != nil {
		slog.Error("PutHostStatus()", "err", err)
		return err
	}
	return nil
}

// ホストの状態情報を収集する
func (m *Marmot) CollectHostStatus() (api.HostStatus, error) {
	var status api.HostStatus
	now := time.Now()
	status.LastUpdated = &now
	if m != nil && m.NodeName != "" {
		slog.Debug("CollectHostStatus()", "NodeName", m.NodeName)
		status.NodeName = util.StringPtr(m.NodeName)
	}
	if hostID := getHostID(); hostID != "" {
		status.HostId = util.StringPtr(hostID)
	}

	// VXLAN underlay は enp2s0 を優先する。
	ipAddress := getHostIPAddressByInterface("enp2s0")
	if ipAddress == "" {
		slog.Warn("enp2s0 address is unavailable; falling back to first active IPv4 address")
		ipAddress = getHostIPAddress()
	}
	status.IpAddress = util.StringPtr(ipAddress)

	// iSCSI イニシエーターID を確保・取得
	nodeName := ""
	if m != nil {
		nodeName = m.NodeName
	}
	if err := ensureISCSIInitiatorID(nodeName); err != nil {
		slog.Warn("ensureISCSIInitiatorID()", "err", err)
	}
	if initiatorID := getISCSIInitiatorID(); initiatorID != "" {
		status.InitiatorId = util.StringPtr(initiatorID)
	}

	// iscsi_server 設定が true の場合は IscsiServer フラグをセット
	if CurrentConfig().IscsiServer {
		status.IscsiServer = util.BoolPtr(true)
	}

	// キャパシティ情報を収集
	capacity, err := collectHostCapacity()
	if err != nil {
		slog.Error("collectHostCapacity()", "err", err)
		return status, err
	}
	status.Capacity = capacity

	// 割当情報を収集
	allocation, err := m.collectHostAllocation()
	if err != nil {
		slog.Error("collectHostAllocation()", "err", err)
		return status, err
	}
	status.Allocation = allocation

	return status, nil
}

// 指定インターフェースのIPv4アドレスを取得する。
func getHostIPAddressByInterface(interfaceName string) string {
	name := strings.TrimSpace(interfaceName)
	if name == "" {
		return ""
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
		return ""
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		return ip.String()
	}

	return ""
}

// ホストの hostid を8桁16進文字列で取得する
func getHostID() string {
	out, err := hostIDCommandOutput()
	if err == nil {
		if id, ok := parseHostIDOutput(out); ok {
			return fmt.Sprintf("%08x", id)
		}
		slog.Warn("hostid command returned an unexpected format", "output", strings.TrimSpace(string(out)))
		return ""
	}
	slog.Warn("hostid command failed", "err", err)
	return ""
}

func parseHostIDOutput(out []byte) (uint32, bool) {
	text := strings.TrimSpace(string(out))
	text = strings.TrimPrefix(strings.ToLower(text), "0x")
	if text == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(text, 16, 32)
	if err != nil || v == 0 {
		return 0, false
	}
	return uint32(v), true
}

// ホスト のIPアドレスを取得する
func getHostIPAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		// ループバックとダウンのインターフェースを除外
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			// IPv4のみを対象とする
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

// iSCSI イニシエーターID を /etc/iscsi/initiatorname.iscsi から取得する
func getISCSIInitiatorID() string {
	data, err := os.ReadFile(iscsiInitiatornameFile)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("Failed to read initiatorname.iscsi", "err", err)
		}
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// コメント行をスキップ
		if strings.HasPrefix(line, "#") {
			continue
		}
		// InitiatorName=xxx の形式を探す
		if strings.HasPrefix(line, "InitiatorName=") {
			initiatorsName := strings.TrimPrefix(line, "InitiatorName=")
			return strings.TrimSpace(initiatorsName)
		}
	}
	return ""
}

// iSCSI イニシエーター名が marmot 形式で設定されていない場合、生成してファイルに書き込み iscsid を再起動する
func ensureISCSIInitiatorID(nodeName string) error {
	existing := getISCSIInitiatorID()
	if strings.HasPrefix(existing, iscsiInitiatornamePrefix) {
		// 既に marmot 形式の IQN が設定されている
		return nil
	}

	// ノード名が空の場合は hostname にフォールバック
	name := strings.TrimSpace(nodeName)
	if name == "" {
		var err error
		name, err = os.Hostname()
		if err != nil || name == "" {
			name = "node"
		}
	}
	newIQN := iscsiInitiatornamePrefix + name

	content := "## Generated by marmot. DO NOT EDIT OR REMOVE THIS FILE!\n" +
		"## If you remove this file, the iSCSI daemon will not start.\n" +
		fmt.Sprintf("InitiatorName=%s\n", newIQN)

	if err := os.MkdirAll(filepath.Dir(iscsiInitiatornameFile), 0755); err != nil {
		return fmt.Errorf("failed to create iscsi dir: %w", err)
	}
	if err := os.WriteFile(iscsiInitiatornameFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write initiatorname.iscsi: %w", err)
	}

	if err := systemctlRestartIscsid(); err != nil {
		return fmt.Errorf("failed to restart iscsid: %w", err)
	}
	slog.Info("iSCSI initiator name configured and iscsid restarted", "iqn", newIQN)
	return nil
}

// ホストのキャパシティ情報を収集する
func collectHostCapacity() (*api.HostCapacity, error) {
	var capacity api.HostCapacity

	// CPUコア数を取得
	cpuCores := runtime.NumCPU()
	capacity.CpuCores = util.IntPtrInt(cpuCores)

	// メモリ搭載量を取得（MB単位）
	var sysinfo syscall.Sysinfo_t
	if err := syscall.Sysinfo(&sysinfo); err != nil {
		slog.Error("syscall.Sysinfo()", "err", err)
	} else {
		totalMemMB := int(sysinfo.Totalram * uint64(sysinfo.Unit) / (1024 * 1024))
		capacity.MemoryMB = util.IntPtrInt(totalMemMB)
	}

	// ネットワークインターフェースを取得
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Error("net.Interfaces()", "err", err)
	} else {
		var nicNames []string
		for _, iface := range ifaces {
			// ループバックを除外
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			nicNames = append(nicNames, iface.Name)
		}
		capacity.NetworkInterfaces = &nicNames
	}

	return &capacity, nil
}

// ホストの割当情報を収集する（etcdのデータから）
func (m *Marmot) collectHostAllocation() (*api.HostAllocation, error) {
	var allocation api.HostAllocation
	currentNode := ""
	if m != nil {
		currentNode = strings.TrimSpace(m.NodeName)
	}

	// サーバー一覧を取得
	servers, err := m.Db.GetServers()
	if err != nil && err != db.ErrNotFound {
		slog.Error("GetServers()", "err", err)
		return nil, err
	}

	totalVMs := 0
	runningVMs := 0
	stoppedVMs := 0
	allocatedCPU := 0
	allocatedMemory := 0

	for _, server := range servers {
		// 割当ノード単位で集計する。これにより各ホストの負荷が分離される。
		if currentNode != "" {
			if server.Metadata.NodeName == nil || strings.TrimSpace(*server.Metadata.NodeName) != currentNode {
				continue
			}
		}

		totalVMs++

		if server.Status == nil {
			continue
		}
		switch server.Status.StatusCode {
		case db.SERVER_RUNNING:
			runningVMs++
			// 稼働中のサーバーのみCPUとメモリを集計
			if server.Spec.Cpu != nil {
				allocatedCPU += *server.Spec.Cpu
			}
			if server.Spec.Memory != nil {
				allocatedMemory += *server.Spec.Memory
			}
		case db.SERVER_STOPPED:
			stoppedVMs++
		}
	}

	allocation.TotalVMs = util.IntPtrInt(totalVMs)
	allocation.RunningVMs = util.IntPtrInt(runningVMs)
	allocation.StoppedVMs = util.IntPtrInt(stoppedVMs)
	allocation.AllocatedCpuCores = util.IntPtrInt(allocatedCPU)
	allocation.AllocatedMemoryMB = util.IntPtrInt(allocatedMemory)

	// 仮想ネットワーク数を取得
	vnets, err := m.Db.GetVirtualNetworks()
	if err != nil && err != db.ErrNotFound {
		slog.Error("GetVirtualNetworks()", "err", err)
		return nil, err
	}
	allocation.VirtualNetworks = util.IntPtrInt(len(vnets))

	return &allocation, nil
}
