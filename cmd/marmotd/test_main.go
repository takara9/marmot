package main

import (
	"log"

	"github.com/labstack/echo/v4"

	api "example.com/m/api"
)

type Server struct {}

func main() {
	e := echo.New()
	server := Server{}
	api.RegisterHandlers(e, server)

	// And we serve HTTP until the world ends.
	log.Fatal(e.Start("0.0.0.0:8080"))
}