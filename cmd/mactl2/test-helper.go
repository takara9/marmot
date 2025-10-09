package main

import (
	"fmt"

	"github.com/labstack/echo/v4"

	"marmot.io/api"
	"marmot.io/marmotd"
)

func startMockServer() *marmotd.Server {
	e := echo.New()
	server := marmotd.NewServer("hvc", "http://127.0.0.1:3379")
	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
	}()
	return server
}
