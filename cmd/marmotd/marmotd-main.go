package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

// 定期的に実行したい関数
func dispatchJobTask() {
	slog.Info("ジョブの要求チェックと実行", "JOB", 0)
}

func startDispatcher() {
	ticker := time.NewTicker(5 * time.Second)
	// TODO: 停止処理を加えること
	go func() {
		for {
			select {
			case <-ticker.C:
				dispatchJobTask()
			}
		}
	}()
}

func main() {
	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:3379", "etcd url")
	flag.Parse()

	// REST-APIサーバーの処理
	e := echo.New()
	Server := marmotd.NewServer(*node, *etcd)
	api.RegisterHandlersWithBaseURL(e, Server, "/api/v1")
	startDispatcher()
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8750"))
}
