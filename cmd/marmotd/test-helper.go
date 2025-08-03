package main

import (
	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

type marmotdServer struct{}

func startMockServer() {
	e := echo.New()
	server := Server{}
	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
	}()
}
