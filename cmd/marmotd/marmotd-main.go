package main

import (
	"context"
	"flag"
	"fmt"
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

	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:3379", "etcd url")
	configPath := flag.String("config", marmotd.DefaultConfigPath, "Path to JSON config file")
	flag.Parse()

	// コンフィグファイルの読み込み
	cfg, err := marmotd.LoadConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load config file", "path", *configPath, "err", err)
		return
	}
	slog.Info("Config loaded", "api_listen_addr", cfg.APIListenAddr,
		"dns_listen_addr", cfg.DNSListenAddr,
		"dns_upstream", cfg.DNSUpstream,
		"deletion_delay_seconds", cfg.DeletionDelaySeconds)

	// REST-APIサーバーの処理
	e := echo.New()
	Server := marmotd.NewServer(*node, *etcd)
	api.RegisterHandlersWithBaseURL(e, Server, "/api/v1")

	// コントローラーの開始

	// 仮想マシンコントローラー
	_, err = controller.StartVmController(*node, *etcd, cfg.DeletionDelaySeconds) // VMコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}
	// ボリュームコントローラー
	_, err = controller.StartVolController(*node, *etcd, cfg.DeletionDelaySeconds) // ボリュームコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}

	// ネットワークコントローラー
	_, err = controller.StartNetController(*node, *etcd, cfg.DeletionDelaySeconds) // ネットワークコントローラーの開始
	if err != nil {
		slog.Error("Failed to start controller", "err", err)
		return
	}

	// DNSサーバーコントローラー
	_, err = internaldns.StartInternalDNSServer(context.Background(), *node, *etcd, cfg) // DNSサーバーコントローラーの開始
	if err != nil {
		slog.Error("Failed to start DNS server", "err", err)
		return
	}

	// イメージコントローラーの開始
	_, err = controller.StartImageController(*node, *etcd, cfg.DeletionDelaySeconds)
	if err != nil {
		slog.Error("Failed to start image controller", "err", err)
		return
	}

	//startDispatcher()
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start(cfg.APIListenAddr))
}
