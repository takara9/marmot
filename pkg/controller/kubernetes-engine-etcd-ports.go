package controller

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/takara9/marmot/api"
)

const (
	// DefaultEtcdPortRangeStart はクラスタ専用etcdのクライアントポート採番の開始値。
	DefaultEtcdPortRangeStart = 23790
	// DefaultEtcdPortRangeEnd はクラスタ専用etcdのクライアントポート採番の終了値(この値は含まない)。
	DefaultEtcdPortRangeEnd = 24790
)

// kubernetesEnginePortProbe は指定ポートがローカルで実際にbind可能(=空き)かどうかを確認する。
// テストから差し替え可能にするためパッケージ変数として保持する。
var kubernetesEnginePortProbe = func(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// parsePortFromEndpoint はURL形式("http://127.0.0.1:2379")や"host:port"形式の
// 文字列からポート番号を取り出す。解釈できない場合は false を返す。
func parsePortFromEndpoint(endpoint string) (int, bool) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return 0, false
	}
	if u, err := url.Parse(trimmed); err == nil && u.Host != "" {
		trimmed = u.Host
	}
	_, portStr, err := net.SplitHostPort(trimmed)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, false
	}
	return port, true
}

// collectReservedKubernetesEngineEtcdPorts は既存クラスタが使用中のetcdクライアント/
// ピアポートと、marmotd自身のetcdクライアントポートを集めた予約済みポート集合を返す。
func collectReservedKubernetesEngineEtcdPorts(engines []api.KubernetesEngine, ownEtcdURL string) map[int]bool {
	reserved := make(map[int]bool)
	if port, ok := parsePortFromEndpoint(ownEtcdURL); ok {
		reserved[port] = true
	}
	for _, ke := range engines {
		if ke.Status == nil {
			continue
		}
		if ke.Status.EtcdClientPort != nil {
			reserved[*ke.Status.EtcdClientPort] = true
		}
		if ke.Status.EtcdPeerPort != nil {
			reserved[*ke.Status.EtcdPeerPort] = true
		}
	}
	return reserved
}

// AllocateKubernetesEngineEtcdPorts は他クラスタ・marmotd自身のetcdと衝突しない
// クライアントポート・ピアポート(隣り合う2ポート)の組を採番する。
// rangeStartからrangeEndの手前まで2ずつ進めながら検索し、両方のポートが未使用かつ
// ローカルで実際にbind可能であるものを返す。見つからない場合はエラーを返す。
func AllocateKubernetesEngineEtcdPorts(engines []api.KubernetesEngine, ownEtcdURL string, rangeStart, rangeEnd int) (clientPort, peerPort int, err error) {
	reserved := collectReservedKubernetesEngineEtcdPorts(engines, ownEtcdURL)
	for base := rangeStart; base+1 < rangeEnd; base += 2 {
		if reserved[base] || reserved[base+1] {
			continue
		}
		if !kubernetesEnginePortProbe(base) || !kubernetesEnginePortProbe(base+1) {
			continue
		}
		return base, base + 1, nil
	}
	return 0, 0, fmt.Errorf("no free etcd client/peer port pair available in range [%d, %d)", rangeStart, rangeEnd)
}
