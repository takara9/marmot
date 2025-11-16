package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

func startMockServer() *marmotd.Server {
	e := echo.New()
	server := marmotd.NewServer("hvc", "http://127.0.0.1:3379")
	go func() {
		// Setup slog
		opts := &slog.HandlerOptions{
			AddSource: true,
			//Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
	}()
	return server
}
