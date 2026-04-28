package marmotd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// StartMockServer はモックサーバーを起動し、サーバーハンドルと goroutine の終了を待つ関数を返す。
// 返された waitDone を cancel() 後に呼び出すことで、シャットダウン goroutine の完了を保証する。
func StartMockServer(ctx context.Context, marmotPort int, etcdPort int) (*Server, func()) {
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

	done := make(chan struct{})

	go func() {
		defer close(done)
		RegisterRoutes(e, server, "/api/v1")

		// サーバー起動
		go func() {
			if err := e.Start("0.0.0.0:" + fmt.Sprintf("%d", marmotPort)); err != nil && err != http.ErrServerClosed {
				fmt.Println("server error:", err)
			}
		}()

		// 停止シグナルを待つ
		<-ctx.Done()

		// シャットダウン
		// context.Background() を使用: ctx はすでにキャンセル済みのため、
		// ctx を親にすると shutdownCtx も即座にキャンセルされ
		// in-flight の HTTP ハンドラーを待てなくなるバグを修正する
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			fmt.Println("shutdown error:", err)
		}
	}()

	waitDone := func() { <-done }
	return server, waitDone
}
