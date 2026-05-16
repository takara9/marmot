package marmotd

import (
	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

// RegisterRoutes registers generated OpenAPI routes and project-specific extension routes.
func RegisterRoutes(e *echo.Echo, server *Server, baseURL string) {
	api.RegisterHandlersWithBaseURL(e, server, baseURL)
	e.GET(baseURL+"/image/:id/qcow2", server.ApiDownloadImageQcow2ById)
}
