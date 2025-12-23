package main

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
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
	e := echo.New()
	server := marmotd.NewServer("hvc", "http://127.0.0.1:3379")

	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")

		// サーバー起動
		go func() {
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
