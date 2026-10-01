package marmotd

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
)

// ManagementDNSForwarderPort は mgmt ネットワーク専用DNSフォワーダーの待受ポート番号(issue #696)。
const ManagementDNSForwarderPort = 53

// managementDNSForwarderTimeout は上位(internal-dns)への問い合わせタイムアウト。
const managementDNSForwarderTimeout = 3 * time.Second

// managementDNSForwarderBufferSize はDNSメッセージ受信用バッファサイズ(EDNS0を考慮し4096バイト)。
const managementDNSForwarderBufferSize = 4096

var (
	managementDNSForwarderMu   sync.Mutex
	managementDNSForwarderConn net.PacketConn
)

// EnsureManagementDNSForwarder は、mgmt-hostポート(10.245.0.1)の準備が整った後に
// ネットワークコントローラーから呼び出され、ゲストVMがmgmtネットワーク経由でDNS解決できるよう
// 10.245.0.1:53 で待ち受けて既存の内部DNSサーバー(dns_listen_addr)へ転送する軽量フォワーダーを
// 起動する(issue #696, docs/MEMO-mgmt-dns.md 案B)。
// 冪等であり、既に起動済みなら何もしない。mgmt-hostインターフェースの準備がまだであるなど、
// bindに失敗した場合はエラーを返す。呼び出し側(ネットワークコントローラーの定期ループ)での
// 再試行に委ね、起動完了までのゲストVMからの問い合わせは一時的な解決失敗を許容する。
func EnsureManagementDNSForwarder(cfg *MarmotdConfig) error {
	managementDNSForwarderMu.Lock()
	defer managementDNSForwarderMu.Unlock()

	if managementDNSForwarderConn != nil {
		return nil
	}

	upstream := strings.TrimSpace(cfg.DNSListenAddr)
	if upstream == "" {
		return fmt.Errorf("dns_listen_addr is empty; cannot start management network dns forwarder")
	}
	if isWildcardDNSListenAddr(upstream) {
		slog.Debug("management network dns is served by the wildcard internal dns listener", "upstream", upstream)
		return nil
	}

	prefix, err := netip.ParsePrefix(ManagementNetworkHostAddress)
	if err != nil {
		return fmt.Errorf("invalid management network host address %q: %w", ManagementNetworkHostAddress, err)
	}
	listenAddr := net.JoinHostPort(prefix.Addr().String(), fmt.Sprintf("%d", ManagementDNSForwarderPort))

	conn, err := startUDPDNSForwarder(listenAddr, upstream)
	if err != nil {
		return fmt.Errorf("failed to start management network dns forwarder on %s: %w", listenAddr, err)
	}

	managementDNSForwarderConn = conn
	slog.Debug("management network dns forwarder started", "listen", listenAddr, "upstream", upstream)
	return nil
}

func isWildcardDNSListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsUnspecified()
}

// managementNetworkNameserversFromConfig は、mgmt NIC用のnameserverを返す(issue #696)。
// EnsureManagementDNSForwarder が Marmotホスト自身の ManagementNetworkHostAddress(10.245.0.1)
// で待ち受けるため、ゲストVMはmgmtネットワーク経由でそのアドレスへ問い合わせる。
func managementNetworkNameserversFromConfig() *api.Nameservers {
	prefix, err := netip.ParsePrefix(ManagementNetworkHostAddress)
	if err != nil {
		slog.Error("invalid ManagementNetworkHostAddress", "value", ManagementNetworkHostAddress, "err", err)
		return nil
	}
	addrs := []string{prefix.Addr().String()}
	return &api.Nameservers{Addresses: &addrs}
}

// startUDPDNSForwarder は listenAddr で待ち受け、受信したDNSクエリをそのまま upstreamAddr へ
// 転送し、応答を問い合わせ元へ中継する単純なUDPプロキシを起動する。
func startUDPDNSForwarder(listenAddr, upstreamAddr string) (net.PacketConn, error) {
	conn, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		return nil, err
	}
	go runUDPDNSForwarderLoop(conn, upstreamAddr)
	return conn, nil
}

// runUDPDNSForwarderLoop は conn への受信を繰り返し、クエリ毎に forwardUDPDNSQuery へ委譲する。
// conn がClose()されるなど読み取りエラー時にループを終了する。
func runUDPDNSForwarderLoop(conn net.PacketConn, upstreamAddr string) {
	buf := make([]byte, managementDNSForwarderBufferSize)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		query := make([]byte, n)
		copy(query, buf[:n])
		go forwardUDPDNSQuery(conn, addr, query, upstreamAddr)
	}
}

// forwardUDPDNSQuery は1件のDNSクエリを upstreamAddr へ転送し、応答を clientAddr へ中継する。
func forwardUDPDNSQuery(conn net.PacketConn, clientAddr net.Addr, query []byte, upstreamAddr string) {
	upstreamConn, err := net.DialTimeout("udp", upstreamAddr, managementDNSForwarderTimeout)
	if err != nil {
		slog.Error("management network dns forwarder: failed to dial upstream", "upstream", upstreamAddr, "err", err)
		return
	}
	defer func() { _ = upstreamConn.Close() }()

	if err := upstreamConn.SetDeadline(time.Now().Add(managementDNSForwarderTimeout)); err != nil {
		slog.Error("management network dns forwarder: failed to set deadline", "err", err)
		return
	}
	if _, err := upstreamConn.Write(query); err != nil {
		slog.Error("management network dns forwarder: failed to forward query", "upstream", upstreamAddr, "err", err)
		return
	}

	respBuf := make([]byte, managementDNSForwarderBufferSize)
	n, err := upstreamConn.Read(respBuf)
	if err != nil {
		slog.Error("management network dns forwarder: failed to read upstream response", "upstream", upstreamAddr, "err", err)
		return
	}

	if _, err := conn.WriteTo(respBuf[:n], clientAddr); err != nil {
		slog.Error("management network dns forwarder: failed to relay response", "client", clientAddr, "err", err)
	}
}
