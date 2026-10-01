package marmotd

import (
	"net"
	"testing"
	"time"
)

// startUDPEchoUpstreamForTest は、受信したクエリに関わらず常に固定レスポンスを返す
// テスト用のダミー上位DNSサーバーを起動する。
func startUDPEchoUpstreamForTest(t *testing.T, response []byte) (addr string, closeFn func()) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test upstream listener: %v", err)
	}
	go func() {
		buf := make([]byte, managementDNSForwarderBufferSize)
		for {
			_, clientAddr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if _, err := conn.WriteTo(response, clientAddr); err != nil {
				return
			}
		}
	}()
	return conn.LocalAddr().String(), func() { _ = conn.Close() }
}

func TestStartUDPDNSForwarder_RelaysQueryAndResponse(t *testing.T) {
	wantResponse := []byte("fake-dns-response")
	upstreamAddr, closeUpstream := startUDPEchoUpstreamForTest(t, wantResponse)
	defer closeUpstream()

	forwarderConn, err := startUDPDNSForwarder("127.0.0.1:0", upstreamAddr)
	if err != nil {
		t.Fatalf("startUDPDNSForwarder() error = %v", err)
	}
	defer func() { _ = forwarderConn.Close() }()

	client, err := net.Dial("udp", forwarderConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("failed to dial forwarder: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Write([]byte("fake-dns-query")); err != nil {
		t.Fatalf("failed to send query: %v", err)
	}

	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}
	buf := make([]byte, managementDNSForwarderBufferSize)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("failed to read relayed response: %v", err)
	}
	if string(buf[:n]) != string(wantResponse) {
		t.Fatalf("relayed response = %q, want %q", buf[:n], wantResponse)
	}
}

func TestStartUDPDNSForwarder_InvalidListenAddrReturnsError(t *testing.T) {
	if _, err := startUDPDNSForwarder("not-a-valid-addr", "127.0.0.1:53"); err == nil {
		t.Fatal("startUDPDNSForwarder() error = nil, want error for invalid listen address")
	}
}

func TestEnsureManagementDNSForwarder_EmptyDNSListenAddrReturnsError(t *testing.T) {
	cfg := &MarmotdConfig{DNSListenAddr: ""}
	if err := EnsureManagementDNSForwarder(cfg); err == nil {
		t.Fatal("EnsureManagementDNSForwarder() error = nil, want error for empty dns_listen_addr")
	}
}

func TestManagementNetworkNameserversFromConfig_ReturnsMgmtHostAddress(t *testing.T) {
	ns := managementNetworkNameserversFromConfig()
	if ns == nil || ns.Addresses == nil {
		t.Fatal("managementNetworkNameserversFromConfig() = nil, want one address")
	}
	addrs := *ns.Addresses
	if len(addrs) != 1 || addrs[0] != "10.245.0.1" {
		t.Fatalf("addresses = %v, want [10.245.0.1]", addrs)
	}
}
