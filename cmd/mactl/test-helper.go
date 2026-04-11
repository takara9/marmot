package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/controller"
	internaldns "github.com/takara9/marmot/pkg/internal-dns"
	"github.com/takara9/marmot/pkg/marmotd"
)

func startMockServer(ctx context.Context) *marmotd.Server {
	/*
		// 個別にログを確認したい場合はコメントアウトを外す
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
	*/
	nodeName := "hvc"
	etcdEp := "http://127.0.0.1:3379"

	e := echo.New()
	server := marmotd.NewServer(nodeName, etcdEp)

	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")

		// サーバー起動
		go func() {
			// コントローラーの開始

			// 仮想マシンコントローラー
			_, err := controller.StartVmController(nodeName, etcdEp, 0) // VMコントローラーの開始
			if err != nil {
				slog.Error("Failed to start controller", "err", err)
				return
			}

			// ボリュームコントローラー
			_, err = controller.StartVolController(nodeName, etcdEp, 0) // ボリュームコントローラーの開始
			if err != nil {
				slog.Error("Failed to start controller", "err", err)
				return
			}

			// ネットワークコントローラー
			_, err = controller.StartNetController(nodeName, etcdEp, 0) // ネットワークコントローラーの開始
			if err != nil {
				slog.Error("Failed to start controller", "err", err)
				return
			}

			// DNSサーバーコントローラー
			_, err = internaldns.StartInternalDNSServer(context.Background(), nodeName, etcdEp, nil) // DNSサーバーコントローラーの開始
			if err != nil {
				slog.Error("Failed to start DNS server", "err", err)
				return
			}

			// イメージコントローラーの開始
			_, err = controller.StartImageController(nodeName, etcdEp, 0)
			if err != nil {
				slog.Error("Failed to start image controller", "err", err)
				return
			}

			if err := e.Start("0.0.0.0:8080"); err != nil && err != http.ErrServerClosed {
				fmt.Println("server error:", err)
			}
		}()

		// 停止シグナルを待つ
		<-ctx.Done()

		// シャットダウン
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			fmt.Println("shutdown error:", err)
		}
	}()

	return server
}

func cleanupTestEnvironment() {
	cmd := exec.Command("lvremove vg1/oslv0900 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg1/oslv0901 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg1/oslv0902 -y")
	cmd.CombinedOutput()

	cmd = exec.Command("lvremove vg2/data0900 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0901 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0902 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0903 -y")
	cmd.CombinedOutput()
}
