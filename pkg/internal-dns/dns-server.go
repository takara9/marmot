package internaldns

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

type controller struct {
	db       *db.Database
	mu       sync.Mutex
	marmot   *marmotd.Marmot
	server   *dns.Server // サーバーインスタンスを保持
	etcdUrl  string
	client   *dns.Client
	Upstream string // 外部DNSサーバーのアドレス (例: "
}

// StartInternalDNSServer はサーバーを非同期で開始し、制御構造体を返します
func StartInternalDNSServer(ctx context.Context, node string, etcdUrl string) (*controller, error) {
	m, err := marmotd.NewMarmot(node, etcdUrl)
	if err != nil {
		slog.Error("Failed to create marmot instance", "err", err)
		return nil, fmt.Errorf("marmot init: %w", err)
	}

	c := &controller{
		marmot:   m,
		db:       m.Db,
		etcdUrl:  etcdUrl,
		Upstream: "8.8.8.8:53",
		client:   &dns.Client{Timeout: 5 * time.Second},
	}

	// DNSサーバーの実体を作成
	mux := dns.NewServeMux()
	mux.HandleFunc(".", c.handleRequest)
	c.server = &dns.Server{
		Addr: "127.0.0.1:53",
		Net:  "udp",
		// Handler を設定。必要に応じて c.handleDNS を実装
		Handler: mux,
	}

	// DNSサーバーを別ゴルーチンで実行
	go c.dnsServer()

	// Graceful Shutdown 用の監視ゴルーチン
	go func() {
		<-ctx.Done() // 外部（main等）からの終了通知を待機
		slog.Info("DNSサーバーのシャットダウンを開始します...")

		// シャットダウンに猶予（タイムアウト）を持たせる
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := c.server.ShutdownContext(shutCtx); err != nil {
			slog.Error("DNSサーバーの強制終了", "err", err)
		}
		slog.Info("DNSサーバーが正常に停止しました")
	}()

	return c, nil
}

func (c *controller) dnsServer() {
	slog.Info("DNSサーバーのリスナーを開始します", "addr", c.server.Addr)

	// ListenAndServe は終了するまでここでブロックされる
	if err := c.server.ListenAndServe(); err != nil {
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
		ipStr := string(resp.Kvs[0].Value)
		ip := net.ParseIP(ipStr[1 : len(ipStr)-1])

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
	slog.Debug("Not found in etcd, forwarding", "q.Name", q.Name)
	reply, _, err := c.client.Exchange(r, c.Upstream)
	if err != nil {
		dns.HandleFailed(w, r)
		return
	}
	w.WriteMsg(reply)
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
