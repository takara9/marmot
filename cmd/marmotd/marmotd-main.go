package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/controller"
	internaldns "github.com/takara9/marmot/pkg/internal-dns"
	"github.com/takara9/marmot/pkg/marmotd"
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
	// Setup slog
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
	slog.Info("Config loaded",
		"node_name", cfg.NodeName,
		"etcd_url", cfg.EtcdURL,
		"api_listen_addr", cfg.APIListenAddr,
		"dns_listen_addr", cfg.DNSListenAddr,
		"dns_upstream", cfg.DNSUpstream,
		"os_volume_group", cfg.OSVolumeGroup,
		"data_volume_group", cfg.DataVolumeGroup,
		"deletion_delay_seconds", cfg.DeletionDelaySeconds,
		"image_create_from_vm_timeout_seconds", cfg.ImageCreateFromVMTimeoutSeconds,
		"image_create_from_url_timeout_seconds", cfg.ImageCreateFromURLTimeoutSeconds,
		"image_download_timeout_seconds", cfg.ImageDownloadTimeoutSeconds,
		"image_resize_timeout_seconds", cfg.ImageResizeTimeoutSeconds,
		"image_delete_timeout_seconds", cfg.ImageDeleteTimeoutSeconds)

	// REST-APIサーバーの処理
	slog.Info("Starting api server", "nodeName", cfg.NodeName, "etcdURL", cfg.EtcdURL, "apiListenAddr", cfg.APIListenAddr)

	e := echo.New()
	slog.Debug("Starting api server #2", "nodeName", cfg.NodeName, "etcdURL", cfg.EtcdURL, "apiListenAddr", cfg.APIListenAddr)
	Server := marmotd.NewServer(cfg.NodeName, cfg.EtcdURL)
	api.RegisterHandlersWithBaseURL(e, Server, "/api/v1")

	// コントローラーの開始
	slog.Info("Starting controllers", "nodeName", cfg.NodeName, "etcdURL", cfg.EtcdURL, "deletionDelaySeconds", cfg.DeletionDelaySeconds)

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

	// DNSサーバーコントローラー
	_, err = internaldns.StartInternalDNSServer(context.Background(), cfg.NodeName, cfg.EtcdURL, cfg) // DNSサーバーコントローラーの開始
	if err != nil {
		slog.Error("Failed to start DNS server", "err", err)
		return
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

	//startDispatcher()
	// And we serve HTTP until the world ends.
	slog.Info("Starting API server", "addr", cfg.APIListenAddr)

	if err := e.Start(cfg.APIListenAddr); err != nil {
		slog.Error("API server stopped", "err", err)
		os.Exit(1)
	}
}
