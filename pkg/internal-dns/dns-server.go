package internaldns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

var errInvalidDNSRecordIP = errors.New("invalid IP address in DNS record")

type controller struct {
	db       *db.Database
	mu       sync.Mutex
	marmot   *marmotd.Marmot
	server   *dns.Server // サーバーインスタンスを保持
	etcdUrl  string
	client   *dns.Client
	Upstream string // 外部DNSサーバーのアドレス (例: "
	allowedUpstreamCIDRs []netip.Prefix
}

// StartInternalDNSServer はサーバーを非同期で開始し、制御構造体を返します。
// cfg に nil を渡した場合はデフォルト設定が使用されます。
func StartInternalDNSServer(ctx context.Context, node string, etcdUrl string, cfg *marmotd.MarmotdConfig) (*controller, error) {
	if cfg == nil {
		var err error
		cfg, err = marmotd.LoadConfig(marmotd.DefaultConfigPath)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	allowedUpstreamCIDRs, err := parseAllowedUpstreamCIDRs(cfg.DNSUpstreamAllowCIDRs)
	if err != nil {
		return nil, fmt.Errorf("parse dns upstream allowlist: %w", err)
	}

	m, err := marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, fmt.Errorf("marmot init: %w", err)
	}

	c := &controller{
		marmot:   m,
		db:       m.Db,
		etcdUrl:  etcdUrl,
		Upstream: cfg.DNSUpstream,
		client:   &dns.Client{Timeout: 5 * time.Second},
		allowedUpstreamCIDRs: allowedUpstreamCIDRs,
	}

	// DNSサーバーの実体を作成
	mux := dns.NewServeMux()
	mux.HandleFunc(".", c.handleRequest)
	c.server = &dns.Server{
		Addr:    cfg.DNSListenAddr,
		Net:     "udp",
		Handler: mux,
	}

	if err := c.startServer(); err != nil {
		return nil, fmt.Errorf("start dns server: %w", err)
	}

	// Graceful Shutdown 用の監視ゴルーチン
	go func() {
		<-ctx.Done() // 外部（main等）からの終了通知を待機
		slog.Debug("DNSサーバーのシャットダウンを開始します...")

		// シャットダウンに猶予（タイムアウト）を持たせる
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := c.server.ShutdownContext(shutCtx); err != nil {
			slog.Error("DNSサーバーの強制終了", "err", err)
		}
		slog.Debug("DNSサーバーが正常に停止しました")
	}()

	return c, nil
}

func (c *controller) startServer() error {
	packetConn, err := net.ListenPacket("udp", c.server.Addr)
	if err != nil {
		return err
	}
	c.server.PacketConn = packetConn

	go c.dnsServer()
	return nil
}

func (c *controller) dnsServer() {
	slog.Debug("DNSサーバーのリスナーを開始します", "addr", c.server.Addr)

	// ActivateAndServe は PacketConn を使って終了するまでここでブロックされる
	if err := c.server.ActivateAndServe(); err != nil {
		// Shutdown による正常終了以外の場合にログを出す
		slog.Error("DNSサーバーが予期せず停止しました", "err", err)
	}
}

func (c *controller) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		slog.Error("Request Handler Error")
		dns.HandleFailed(w, r)
		return
	}

	q := r.Question[0]
	// 末尾のドットを除去してetcdのキーを作成 (example.com. -> /dns/example.com)
	etcdKey := DomainToMarmotPath(q.Name)

	// etcd から IP アドレスを取得
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := c.db.Cli.Get(ctx, etcdKey)
	if err != nil {
		slog.Error("検索の失敗 Failed to query etcd", "err", err)
		dns.HandleFailed(w, r)
		return
	}

	if len(resp.Kvs) > 0 {
		ip, ipStr, err := decodeDNSRecordIP(resp.Kvs[0].Value)
		if err != nil {
			slog.Error("Failed to decode DNS record", "err", err, "key", etcdKey)
			dns.HandleFailed(w, r)
			return
		}

		if ip != nil && q.Qtype == dns.TypeA {
			log.Printf("Resolved from etcd: %s -> %s", q.Name[:len(q.Name)-1], ipStr)
			m := new(dns.Msg)
			m.SetReply(r)
			m.Answer = append(m.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   ip,
			})
			w.WriteMsg(m)
			return
		}
	}

	// etcd にない場合は外部へ転送
	if !shouldForwardUpstream(w.RemoteAddr(), c.allowedUpstreamCIDRs) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = dns.RcodeRefused
		w.WriteMsg(m)
		return
	}

	slog.Debug("Not found in etcd, forwarding", "q.Name", q.Name)
	reply, _, err := c.client.Exchange(r, c.Upstream)
	if err != nil {
		dns.HandleFailed(w, r)
		return
	}
	w.WriteMsg(reply)
}

func decodeDNSRecordIP(raw []byte) (net.IP, string, error) {
	var ipStr string
	if err := json.Unmarshal(raw, &ipStr); err != nil {
		return nil, "", err
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, ipStr, errInvalidDNSRecordIP
	}
	return ip, ipStr, nil
}

func parseAllowedUpstreamCIDRs(cidrs []string) ([]netip.Prefix, error) {
	if len(cidrs) == 0 {
		return nil, nil
	}

	allowed := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", cidr, err)
		}
		allowed = append(allowed, prefix.Masked())
	}

	return allowed, nil
}

func shouldForwardUpstream(remoteAddr net.Addr, allowedCIDRs []netip.Prefix) bool {
	if remoteAddr == nil {
		return false
	}

	host, _, err := net.SplitHostPort(remoteAddr.String())
	if err != nil {
		host = remoteAddr.String()
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}

	if addr.IsLoopback() {
		return true
	}

	for _, prefix := range allowedCIDRs {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

// DomainToMarmotPath はドメイン名を /marmot/dns/ 形式のパスに変換します
func DomainToMarmotPath(domain string) string {
	// 末尾のドットを削除し、ドットで分割
	domain = strings.TrimSuffix(domain, ".")
	parts := strings.Split(domain, ".")

	// スライスの要素を逆順に入れ替え
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	// プレフィックス を先頭につけて結合
	return db.InternalDNSPrefix + "/" + strings.Join(parts, "/")
}
