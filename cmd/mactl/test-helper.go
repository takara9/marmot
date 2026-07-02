package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/pkg/controller"
	internaldns "github.com/takara9/marmot/pkg/internal-dns"
	"github.com/takara9/marmot/pkg/marmotd"
)

type mockServerHandle struct {
	server *marmotd.Server
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

var findMockServerConfigPathFn = findMockServerConfigPath
var defaultMockServerConfigFn = defaultMockServerConfig

func ensureMactlTestBinary() error {
	if _, err := os.Stat("bin/mactl-test"); err == nil {
		return nil
	}

	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	cmd := exec.Command("go", "build", "-o", "bin/mactl-test", ".")
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build bin/mactl-test: %w (output=%s)", err, string(output))
	}

	return nil
}

func loginAsAdmin() error {
	cmd := exec.Command("sh", "-lc", "printf 'passw0rd\n' | script -qec './bin/mactl-test --api testdata/.marmot login admin' /dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to login as admin: %w (output=%s)", err, string(output))
	}
	return nil
}

func setupMactlTestHome() (string, error) {
	homeDir, err := os.MkdirTemp("", "mactl-home-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp HOME: %w", err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		_ = os.RemoveAll(homeDir)
		return "", fmt.Errorf("failed to set HOME: %w", err)
	}
	return homeDir, nil
}

func startEtcdContainer() (string, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("failed to allocate free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%d:2379", port), "ghcr.io/takara9/etcd:3.6.5")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("failed to start etcd container: %s, %w", string(output), err)
	}

	containerID := strings.TrimSpace(string(output))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	return containerID, fmt.Sprintf("http://127.0.0.1:%d", port), nil
}

func findMockServerConfigPath() (string, []string, error) {
	candidates := make([]string, 0, 24)
	seen := map[string]struct{}{}
	addCandidate := func(baseDir, rel string) {
		if strings.TrimSpace(rel) == "" {
			return
		}
		path := ""
		if strings.TrimSpace(baseDir) == "" {
			path = filepath.Clean(rel)
		} else {
			path = filepath.Clean(filepath.Join(baseDir, rel))
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}

	if envPath := strings.TrimSpace(os.Getenv("MARMOT_MOCK_SERVER_CONFIG")); envPath != "" {
		addCandidate("", envPath)
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		srcDir := filepath.Dir(file)
		addCandidate(srcDir, "testdata/marmotd.json")
		addCandidate(filepath.Clean(filepath.Join(srcDir, "../..")), "cmd/mactl/testdata/marmotd.json")
	}

	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 12; i++ {
			addCandidate(dir, "testdata/marmotd.json")
			addCandidate(dir, "cmd/mactl/testdata/marmotd.json")

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, p := range candidates {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, candidates, nil
		}
	}

	return "", candidates, fmt.Errorf("failed to find mock server config")
}

func defaultMockServerConfig() (*marmotd.MarmotdConfig, error) {
	// 存在しないパスを指定して、marmotd 側のデフォルト設定を取得する。
	baseCfg, err := marmotd.LoadConfig(filepath.Join(os.TempDir(), "marmotd-mock-defaults-do-not-create.json"))
	if err != nil {
		return nil, err
	}

	netmasklen := 24
	defaultRouteTo := "default"
	defaultRouteVia := "192.168.1.1"
	baseCfg.HostBridgeIPNetAddr = "192.168.1.0/24"
	baseCfg.HostBridgeIPAddrStart = "192.168.1.190"
	baseCfg.HostBridgeIPAddrEnd = "192.168.1.194"
	baseCfg.HostBridgeDefault = &marmotd.HostBridgeDefault{
		Netmasklen: &netmasklen,
		Nameservers: &api.Nameservers{
			Addresses: &[]string{"8.8.8.8"},
			Search:    &[]string{"labo.local"},
		},
		Routes: &[]api.Route{{
			To:  &defaultRouteTo,
			Via: &defaultRouteVia,
		}},
	}

	return baseCfg, nil
}

func loadMockServerConfig(etcdEp string) (*marmotd.MarmotdConfig, error) {
	cfgPath, triedPaths, err := findMockServerConfigPathFn()
	if err != nil {
		cfg, fallbackErr := defaultMockServerConfigFn()
		if fallbackErr != nil {
			return nil, fmt.Errorf("failed to find mock server config: tried %v (fallback error: %v)", triedPaths, fallbackErr)
		}
		slog.Warn("mock server config file not found; using built-in defaults", "tried", triedPaths)
		cfg.EtcdURL = etcdEp
		cfg.NodeName = "hvc"
		cfg.DNSListenAddr = "127.0.0.1:1053"
		marmotd.SetRuntimeConfig(cfg)
		return cfg, nil
	}

	cfg, err := marmotd.LoadConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load mock server config %s: %w", cfgPath, err)
	}

	// テスト用の起動環境に合わせて runtime config を補正する。
	cfg.EtcdURL = etcdEp
	cfg.NodeName = "hvc"
	cfg.DNSListenAddr = "127.0.0.1:1053"
	if strings.TrimSpace(cfg.HostBridgeIPNetAddr) == "" || strings.TrimSpace(cfg.HostBridgeIPAddrStart) == "" || strings.TrimSpace(cfg.HostBridgeIPAddrEnd) == "" {
		return nil, fmt.Errorf("mock server config %s is missing host-bridge IPAM settings", cfgPath)
	}
	marmotd.SetRuntimeConfig(cfg)

	return cfg, nil
}

func startMockServer(etcdEp string) (*mockServerHandle, error) {
	// 個別にログを確認したい場合はコメントアウトを外す
	//opts := &slog.HandlerOptions{
	//	AddSource: true,
	//	Level:     slog.LevelDebug,
	//}
	//logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	//slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	h := &mockServerHandle{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	cfg, err := loadMockServerConfig(etcdEp)
	if err != nil {
		cancel()
		return nil, err
	}
	nodeName := cfg.NodeName

	e := echo.New()
	server := marmotd.NewServerWithOptions(nodeName, etcdEp, true)
	h.server = server

	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		defer close(h.done)
		marmotd.RegisterRoutes(e, server, "/api/v1")

		type stopper interface{ Stop() }
		stoppers := make([]stopper, 0, 7)
		stopControllers := func() {
			for i := len(stoppers) - 1; i >= 0; i-- {
				stoppers[i].Stop()
			}
		}

		// コントローラーの開始
		vmController, err := controller.StartVmController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start VM controller", "err", err)
			errCh <- err
			return
		}
		stoppers = append(stoppers, vmController)

		volController, err := controller.StartVolController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start volume controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, volController)

		netController, err := controller.StartNetController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start network controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, netController)

		gatewayController, err := controller.StartGatewayController(nodeName, etcdEp)
		if err != nil {
			slog.Error("Failed to start gateway controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, gatewayController)

		dnsCfg := &marmotd.MarmotdConfig{
			DNSListenAddr: "127.0.0.1:1053",
			DNSUpstream:   "8.8.8.8:53",
		}
		_, err = internaldns.StartInternalDNSServer(ctx, nodeName, etcdEp, dnsCfg)
		if err != nil {
			slog.Error("Failed to start DNS server", "err", err)
			stopControllers()
			errCh <- err
			return
		}

		imageController, err := controller.StartImageController(nodeName, etcdEp, 0)
		if err != nil {
			slog.Error("Failed to start image controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, imageController)

		hostController, err := controller.StartHostController(nodeName, etcdEp)
		if err != nil {
			slog.Error("Failed to start host controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, hostController)

		schedulerController, err := controller.StartSchedulerController(nodeName, etcdEp)
		if err != nil {
			slog.Error("Failed to start scheduler controller", "err", err)
			stopControllers()
			errCh <- err
			return
		}
		stoppers = append(stoppers, schedulerController)

		serverErrCh := make(chan error, 1)
		go func() {
			serverErrCh <- e.Start("0.0.0.0:8080")
		}()

		close(readyCh)

		<-ctx.Done()
		stopControllers()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			fmt.Println("shutdown error:", err)
		}

		select {
		case err := <-serverErrCh:
			if err != nil && err != http.ErrServerClosed {
				fmt.Println("server error:", err)
			}
		default:
		}
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		cancel()
		<-h.done
		return nil, fmt.Errorf("mock server startup failed: %w", err)
	case <-time.After(10 * time.Second):
		cancel()
		<-h.done
		return nil, errors.New("mock server startup timeout")
	}

	if err := waitMockServerReady(30 * time.Second); err != nil {
		h.Stop()
		return nil, err
	}

	return h, nil
}

func (h *mockServerHandle) Stop() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		if h.cancel != nil {
			h.cancel()
		}
	})
	if h.done != nil {
		<-h.done
	}
}

func waitMockServerReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	readinessURL := "http://127.0.0.1:8080/api/v1/ping"

	for time.Now().Before(deadline) {
		resp, err := client.Get(readinessURL)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return errors.New("mock server readiness timeout")
}

func cleanupTestEnvironment() {
	// データのクリア
}
