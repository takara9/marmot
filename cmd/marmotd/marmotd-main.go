package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

func main() {
	node := flag.String("node", "hv1", "Hypervisor node name")
	etcd := flag.String("etcd", "http://127.0.0.1:3379", "etcd url")
	flag.Parse()
	fmt.Println("node = ", *node)
	fmt.Println("etcd = ", *etcd)

	// Setup slog
	opts := &slog.HandlerOptions{
		AddSource: true,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
	slog.SetDefault(logger)
	e := echo.New()

	Server := marmotd.NewServer(*node, *etcd)

	api.RegisterHandlersWithBaseURL(e, Server, "/api/v1")
	// And we serve HTTP until the world ends.
	fmt.Println(e.Start("0.0.0.0:8750"))
}
