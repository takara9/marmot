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

const testMockHostIDOutput = "0x00000001\n"

var hostIDCommandOutput = func() ([]byte, error) {
	return exec.Command("hostid").Output()
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

	// IPアドレスを取得
	ipAddress := getHostIPAddress()
	status.IpAddress = util.StringPtr(ipAddress)

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

// ホストのIPアドレスを取得する
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
			if server.Metadata == nil || server.Metadata.NodeName == nil || strings.TrimSpace(*server.Metadata.NodeName) != currentNode {
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
			if server.Spec != nil {
				if server.Spec.Cpu != nil {
					allocatedCPU += *server.Spec.Cpu
				}
				if server.Spec.Memory != nil {
					allocatedMemory += *server.Spec.Memory
				}
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
