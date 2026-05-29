package marmotd

import (
	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

// RegisterRoutes registers generated OpenAPI routes and project-specific extension routes.
func RegisterRoutes(e *echo.Echo, server *Server, baseURL string) {
	api.RegisterHandlersWithBaseURL(e, server, baseURL)
	e.GET(baseURL+"/image/:id/qcow2", server.ApiDownloadImageQcow2ById)
	e.GET(baseURL+"/gateway/:id/cert", func(ctx echo.Context) error {
		return server.ApiGetGatewayCertById(ctx, ctx.Param("id"))
	})
	e.POST(baseURL+"/vpn-gateway", server.ApiCreateVpnGateway)
	e.GET(baseURL+"/vpn-gateway", server.ApiGetVpnGateways)
	e.GET(baseURL+"/vpn-gateway/:id", func(ctx echo.Context) error {
		return server.ApiGetVpnGatewayById(ctx, ctx.Param("id"))
	})
	e.PUT(baseURL+"/vpn-gateway/:id", func(ctx echo.Context) error {
		return server.ApiUpdateVpnGatewayById(ctx, ctx.Param("id"))
	})
	e.DELETE(baseURL+"/vpn-gateway/:id", func(ctx echo.Context) error {
		return server.ApiDeleteVpnGatewayById(ctx, ctx.Param("id"))
	})
	e.GET(baseURL+"/vpn-gateway/:id/cert", func(ctx echo.Context) error {
		return server.ApiGetVpnGatewayCertById(ctx, ctx.Param("id"))
	})
}
