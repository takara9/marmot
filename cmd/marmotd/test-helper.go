package main

import (
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

func startMockServer() *Server {
	e := echo.New()
	server := NewServer("hvc", "http://127.0.0.1:3379")
	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
	}()
	return server
}
