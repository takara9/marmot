package main

import (
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

func startMockServer() {
	e := echo.New()
	server := Server{}
	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
	}()
}
