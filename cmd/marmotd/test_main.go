package main

import (
	"log"

	"github.com/labstack/echo/v4"

	api "example.com/m/api"
)

type Server struct {}

// GetPong implements api.ServerInterface.
func (s Server) GetPong(ctx echo.Context) error {
	return ctx.String(200, "pong")
}

func main() {
	e := echo.New()
	server := Server{}
	api.RegisterHandlers(e, server)

	// And we serve HTTP until the world ends.
	log.Fatal(e.Start("0.0.0.0:8080"))
}