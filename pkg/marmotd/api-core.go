package marmotd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"
)

//go:embed version.txt
var Version string

// DEBUG Print
const DEBUG bool = true

type Marmot struct {
	NodeName string
	EtcdUrl  string
	Db       *db.Database
	Virt     *virt.LibVirtEp
}

type Server struct {
	Lock          sync.Mutex
	Ma            *Marmot
	cleanupCancel context.CancelFunc
	cleanupDone   chan struct{}
}

const (
	idleLoginSessionRevokeInterval   = 1 * time.Minute
	revokedAPIKeyCleanupInterval    = 1 * time.Minute
	revokedAPIKeyPhysicalDeleteAfter = 1 * time.Minute
)

var (
	netDialTimeout       = net.DialTimeout
	ovsLookPath          = exec.LookPath
	ovsShowCommand       = runOVSShow
	listenPacketForCheck = net.ListenPacket
)

// Marmot インスタンスの生成、これにより関数コールが可能となる
// etcdUrl は、etcd サーバーの URL を指定する
// nodeName は、ハイパーバイザーのノード名を指定する
func NewMarmot(nodeName string, etcdUrl string) (*Marmot, error) {
	var m Marmot
	var err error
	m.Db, err = db.NewDatabase(etcdUrl)
	if err != nil {
		slog.Error("Failed to initialize database", "err", err)
		return nil, err
	}
	m.NodeName = nodeName
	m.EtcdUrl = etcdUrl
	m.Virt, err = virt.NewLibVirtEp("qemu:///system")
	if err != nil {
		slog.Error("Failed to initialize libvirt endpoint", "err", err)
		return nil, err
	}
	return &m, nil
}

func ValidateStartupPrerequisites(etcdURL, dnsListenAddr string) error {
	var issues []string

	if err := validateEtcdReachable(etcdURL); err != nil {
		issues = append(issues, err.Error())
	}
	if err := validateOVSReady(); err != nil {
		issues = append(issues, err.Error())
	}
	if err := validateDNSListenAddrAvailable(dnsListenAddr); err != nil {
		issues = append(issues, err.Error())
	}

	if len(issues) > 0 {
		return fmt.Errorf("startup preflight failed: %s", strings.Join(issues, "; "))
	}

	return nil
}

func validateEtcdReachable(etcdURL string) error {
	addr := strings.TrimSpace(etcdURL)
	if addr == "" {
		return fmt.Errorf("etcd endpoint is empty")
	}

	hostPort := strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
	if !strings.Contains(hostPort, ":") {
		hostPort = net.JoinHostPort(hostPort, "2379")
	}

	conn, err := netDialTimeout("tcp", hostPort, 2*time.Second)
	if err != nil {
		return fmt.Errorf("etcd is not reachable at %s: %w", hostPort, err)
	}
	_ = conn.Close()
	return nil
}

func validateOVSReady() error {
	if _, err := ovsLookPath("ovs-vsctl"); err != nil {
		return fmt.Errorf("ovs-vsctl is not available: %w", err)
	}

	if err := ovsShowCommand(); err != nil {
		return fmt.Errorf("openvswitch is not ready (ovs-vsctl show failed): %w", err)
	}

	return nil
}

func runOVSShow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ovs-vsctl", "show")
	if output, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("timeout")
		}
		return fmt.Errorf("%w (output=%s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func validateDNSListenAddrAvailable(dnsListenAddr string) error {
	addr := strings.TrimSpace(dnsListenAddr)
	if addr == "" {
		return nil
	}

	conn, err := listenPacketForCheck("udp", addr)
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("dns listen address %s is already in use (possible port conflict)", addr)
		}
		return fmt.Errorf("failed to check dns listen address %s: %w", addr, err)
	}
	_ = conn.Close()
	return nil
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.EADDRINUSE) {
		return true
	}
	return false
}

// Marmot インスタンスの終了
func (m *Marmot) Close() error {
	if m.Virt != nil {
		m.Virt.Close()
	}
	if m.Db != nil {
		if err := m.Db.Close(); err != nil {
			slog.Error("Failed to close database", "err", err)
			return err
		}
	}
	return nil
}

// marmotd サーバーの生成、REST API サーバーを起動する
// marmotdで定義された関数に対して、REST API 経由でアクセスできるようにする
func NewServer(node string, etcdurl string) *Server {
	return NewServerWithOptions(node, etcdurl, false)
}

func NewServerWithOptions(node string, etcdurl string, skipPreflight bool) *Server {
	if !skipPreflight {
		if err := ValidateStartupPrerequisites(etcdurl, CurrentConfig().DNSListenAddr); err != nil {
			slog.Error("Startup preflight check failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Debug("Startup preflight check is skipped", "node", node, "etcd", etcdurl)
	}

	marmotInstance, err := NewMarmot(node, etcdurl)
	if err != nil {
		slog.Error("Failed to initialize Marmot core", "err", err)
		os.Exit(1)
	}
	if err := marmotInstance.Db.EnsureBootstrapAdmin(); err != nil {
		slog.Error("Failed to seed bootstrap admin user", "err", err)
		os.Exit(1)
	}
	s := &Server{
		Ma: marmotInstance,
	}
	s.startRevokedAPIKeyCleanupWorker()
	return s
}

// サーバーの終了
func (s *Server) Close() error {
	if s.cleanupCancel != nil {
		s.cleanupCancel()
	}
	if s.cleanupDone != nil {
		select {
		case <-s.cleanupDone:
		case <-time.After(3 * time.Second):
			slog.Warn("Timed out waiting for revoked API key cleanup worker to stop")
		}
	}
	return s.Ma.Db.Close()
}

func (s *Server) startRevokedAPIKeyCleanupWorker() {
	if s == nil || s.Ma == nil || s.Ma.Db == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cleanupCancel = cancel
	s.cleanupDone = make(chan struct{})

	go func() {
		defer close(s.cleanupDone)
		revokeTicker := time.NewTicker(idleLoginSessionRevokeInterval)
		defer revokeTicker.Stop()
		ticker := time.NewTicker(revokedAPIKeyCleanupInterval)
		defer ticker.Stop()

		slog.Info("Started API key maintenance worker", "revokeInterval", idleLoginSessionRevokeInterval.String(), "cleanupInterval", revokedAPIKeyCleanupInterval.String(), "deleteAfter", revokedAPIKeyPhysicalDeleteAfter.String())

		for {
			select {
			case <-ctx.Done():
				return
			case <-revokeTicker.C:
				revoked, err := s.Ma.Db.RevokeIdleLoginSessionsOlderThan(db.AuthSessionIdleTimeout)
				if err != nil {
					slog.Warn("Idle login session revocation failed", "err", err, "revoked", revoked)
					continue
				}
				if revoked > 0 {
					slog.Info("Idle login session revocation completed", "revoked", revoked)
				}
			case <-ticker.C:
				deleted, err := s.Ma.Db.CleanupRevokedApiKeysOlderThan(revokedAPIKeyPhysicalDeleteAfter)
				if err != nil {
					slog.Warn("Revoked API key cleanup failed", "err", err, "deleted", deleted)
					continue
				}
				if deleted > 0 {
					slog.Info("Revoked API key cleanup completed", "deleted", deleted)
				}
			}
		}
	}()
}

// ＝＝＝＝＝＝＝＝＝＝＝＝＝＝　API 関数群  ＝＝＝＝＝＝＝＝＝＝＝＝＝＝
// 生存確認
func (s *Server) ApiReplyPing(ctx echo.Context) error {
	slog.Debug("===", "ReplyPing() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()
	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "ok"})
}

// バージョン取得
func (s *Server) ApiGetVersion(ctx echo.Context) error {
	slog.Debug("===", "GetVersion() is called", "===")
	s.Lock.Lock()
	defer s.Lock.Unlock()
	var v api.Version
	v.ServerVersion = &Version
	return ctx.JSON(http.StatusOK, v)
}
