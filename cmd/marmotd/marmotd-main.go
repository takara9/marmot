package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/pkg/controller"
	internaldns "github.com/takara9/marmot/pkg/internal-dns"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

var controllerCounter uint64 = 0

// 定期的に実行したい関数
//func dispatchJobTask() {
//	slog.Debug("ジョブの要求チェックと実行", "JOB", 0)
//}

//func startDispatcher() {
//	ticker := time.NewTicker(15 * time.Second)
// TODO: 停止処理を加えること
//	go func() {
//		for {
//			select {
//			case <-ticker.C:
//				dispatchJobTask()
//			}
//		}
//	}()
//}

func main() {
	// LoadConfig 失敗時のエラー出力用の仮ロガー。
	// SetRuntimeConfig 後に SetupDefaultLogger で Loki 対応ロガーに置き換えられる。
	opts := &slog.HandlerOptions{
		AddSource: true,
		//Level:     slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	configPath := flag.String("config", marmotd.DefaultConfigPath, "Path to JSON config file")
	flag.Parse()

	// コンフィグファイルの読み込み
	cfg, err := marmotd.LoadConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load config file", "path", *configPath, "err", err)
		return
	}
	// クラスタ構成では設定ファイルより OS のホスト名を優先する。
	hostname, err := os.Hostname()
	if err != nil {
		slog.Warn("Failed to get OS hostname, fallback to configured node name", "configured_node_name", cfg.NodeName, "err", err)
	} else if hostname != "" {
		cfg.NodeName = hostname
	}
	marmotd.SetRuntimeConfig(cfg)
	logShutdown, err := marmotd.SetupDefaultLogger(cfg)
	if err != nil {
		slog.Warn("Failed to initialize Loki logger; continuing with stderr logger", "err", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := logShutdown(ctx); err != nil {
			slog.Warn("Failed to flush logger on shutdown", "err", err)
		}
	}()

	if err := marmotd.EnsureGatewayRuntimeAssets(); err != nil {
		slog.Error("Failed to initialize gateway runtime assets", "err", err)
		return
	}
	slog.Debug("Config loaded",
		"node_name", cfg.NodeName,
		"etcd_url", cfg.EtcdURL,
		"api_listen_addr", cfg.APIListenAddr,
		"dns_listen_addr", cfg.DNSListenAddr,
		"dns_upstream", cfg.DNSUpstream,
		"dns_upstream_allow_cidrs", cfg.DNSUpstreamAllowCIDRs,
		"os_volume_group", cfg.OSVolumeGroup,
		"data_volume_group", cfg.DataVolumeGroup,
		"deletion_delay_seconds", cfg.DeletionDelaySeconds,
		"image_create_from_vm_timeout_seconds", cfg.ImageCreateFromVMTimeoutSeconds,
		"image_create_from_url_timeout_seconds", cfg.ImageCreateFromURLTimeoutSeconds,
		"image_download_timeout_seconds", cfg.ImageDownloadTimeoutSeconds,
		"image_resize_timeout_seconds", cfg.ImageResizeTimeoutSeconds,
		"image_delete_timeout_seconds", cfg.ImageDeleteTimeoutSeconds,
		"loki_push_url", cfg.LokiPushURL)

	// Setup host-bridge for libvirt
	util.SetupHostBridge()

	// REST-APIサーバーの処理
	slog.Debug("Starting api server", "nodeName", cfg.NodeName, "etcdURL", cfg.EtcdURL, "apiListenAddr", cfg.APIListenAddr)

	e := echo.New()
	slog.Debug("Starting api server #2", "nodeName", cfg.NodeName, "etcdURL", cfg.EtcdURL, "apiListenAddr", cfg.APIListenAddr)
	Server := marmotd.NewServer(cfg.NodeName, cfg.EtcdURL)
	telemetry, err := marmotd.RegisterOpenTelemetryMetrics(e, Server.Ma.Db)
	if err != nil {
		slog.Error("Failed to initialize OpenTelemetry metrics", "err", err)
		return
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(ctx); err != nil {
			slog.Warn("Failed to shutdown telemetry", "err", err)
		}
	}()

	if err := Server.Ma.CollectAndUpdateHostStatus(); err != nil {
		slog.Warn("Failed to collect initial host status before OVN bootstrap", "err", err)
	}
	if err := marmotd.EnsureOVNRuntimeBootstrap(Server.Ma, cfg); err != nil {
		slog.Warn("Failed to ensure OVN runtime bootstrap", "err", err)
	}
	marmotd.RegisterRoutes(e, Server, "/api/v1")

	// コントローラーの開始
	slog.Debug("Starting controllers", "nodeName", cfg.NodeName, "etcdURL", cfg.EtcdURL, "deletionDelaySeconds", cfg.DeletionDelaySeconds)

	// 仮想マシンコントローラー
	_, err = controller.StartVmController(cfg.NodeName, cfg.EtcdURL, cfg.DeletionDelaySeconds) // VMコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}
	// ボリュームコントローラー
	_, err = controller.StartVolController(cfg.NodeName, cfg.EtcdURL, cfg.DeletionDelaySeconds) // ボリュームコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}

	// ネットワークコントローラー
	_, err = controller.StartNetController(cfg.NodeName, cfg.EtcdURL, cfg.DeletionDelaySeconds) // ネットワークコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}

	// ゲートウェイコントローラー
	_, err = controller.StartGatewayController(cfg.NodeName, cfg.EtcdURL)
	if err != nil {
		slog.Error("Failed to start gateway controller", "err", err)
		return
	}

	// VPNゲートウェイコントローラー
	_, err = controller.StartVpnGatewayController(cfg.NodeName, cfg.EtcdURL)
	if err != nil {
		slog.Error("Failed to start vpn gateway controller", "err", err)
		return
	}

	// アプリケーションロードバランサーコントローラー
	_, err = controller.StartApplicationLoadBalancerController(cfg.NodeName, cfg.EtcdURL)
	if err != nil {
		slog.Error("Failed to start application load balancer controller", "err", err)
		return
	}

	// ネットワークロードバランサーコントローラー
	_, err = controller.StartNetworkLoadBalancerController(cfg.NodeName, cfg.EtcdURL)
	if err != nil {
		slog.Error("Failed to start network load balancer controller", "err", err)
		return
	}

	// DNSサーバーコントローラー
	_, err = internaldns.StartInternalDNSServer(context.Background(), cfg.NodeName, cfg.EtcdURL, cfg) // DNSサーバーコントローラーの開始
	if err != nil {
		slog.Error("Failed to start DNS server", "err", err)
		return
	}

	// ローカルリゾルバーの初期化（内部DNSの起動成功後に切り替える）
	if err := util.SetupLocalResolver(cfg.DNSListenAddr); err != nil {
		slog.Error("Failed to setup local resolver", "err", err)
		return
	}

	// Provision OS images from configuration
	if err := marmotd.ProvisionOSImages(Server.Ma, cfg.OSImages); err != nil {
		slog.Warn("OS image provisioning encountered an error", "err", err)
		// Continue startup even if provisioning fails
	}

	// イメージコントローラーの開始
	_, err = controller.StartImageController(cfg.NodeName, cfg.EtcdURL, cfg.DeletionDelaySeconds)
	if err != nil {
		slog.Error("Failed to start image controller", "err", err)
		return
	}

	// ホストコントローラーの開始
	_, err = controller.StartHostController(cfg.NodeName, cfg.EtcdURL)
	if err != nil {
		slog.Error("Failed to start host controller", "err", err)
		return
	}

	// スケジューラーコントローラーの開始
	_, err = controller.StartSchedulerController(cfg.NodeName, cfg.EtcdURL)
	if err != nil {
		slog.Error("Failed to start scheduler controller", "err", err)
		return
	}

	//startDispatcher()
	// And we serve HTTP(S) until the world ends.
	slog.Debug("Starting API server", "addr", cfg.APIListenAddr)

	// Use TLS if both cert and key are configured, otherwise use HTTP.
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		slog.Info("Starting API server with TLS", "addr", cfg.APIListenAddr, "cert", cfg.TLSCertFile, "key", cfg.TLSKeyFile)
		if err := e.StartTLS(cfg.APIListenAddr, cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
			slog.Error("API server stopped", "err", err)
			return
		}
	} else {
		slog.Warn("Starting API server without TLS (no cert/key configured)", "addr", cfg.APIListenAddr)
		if err := e.Start(cfg.APIListenAddr); err != nil {
			slog.Error("API server stopped", "err", err)
			return
		}
	}
}
