package marmotd

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

func StartMockServer(ctx context.Context, marmotPort int, etcdPort int) *Server {
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
	server := NewServer("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))

	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")

		// サーバー起動
		go func() {
			if err := e.Start("0.0.0.0:" + fmt.Sprintf("%d", marmotPort)); err != nil && err != http.ErrServerClosed {
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

func CleanupTestEnvironment() {
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

	cmd = exec.Command("docker kill $(docker ps |awk 'NR>1 {print $1}')")
	cmd.CombinedOutput()

	cmd = exec.Command("docker rm $(docker ps --all |awk 'NR>1 {print $1}')")
	cmd.CombinedOutput()
}
