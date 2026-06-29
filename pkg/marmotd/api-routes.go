package marmotd

import (
	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

// RegisterRoutes registers generated OpenAPI routes and project-specific extension routes.
func RegisterRoutes(e *echo.Echo, server *Server, baseURL string) {
	operationMiddlewares := rbacOperationMiddlewares(server)
	api.RegisterHandlersWithOptions(e, server, api.RegisterHandlersOptions{
		BaseURL:              baseURL,
		OperationMiddlewares: operationMiddlewares,
	})
	e.GET(baseURL+"/image/:id/qcow2", server.ApiDownloadImageQcow2ById, operationMiddlewares["apiDownloadImageQcow2ById"]...)
	e.POST(baseURL+"/image/import", server.ApiImportImageArchive, operationMiddlewares["apiImportImageArchive"]...)
	e.GET(baseURL+"/gateway/:id/cert", func(ctx echo.Context) error {
		return server.ApiGetGatewayCertById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiGetGatewayCertById"]...)
	e.POST(baseURL+"/application-load-balancer", server.ApiCreateLoadBalancer, operationMiddlewares["apiCreateLoadBalancer"]...)
	e.GET(baseURL+"/application-load-balancer", server.ApiGetLoadBalancers, operationMiddlewares["apiGetLoadBalancers"]...)
	e.GET(baseURL+"/application-load-balancer/:id", func(ctx echo.Context) error {
		return server.ApiGetLoadBalancerById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiGetLoadBalancerById"]...)
	e.PUT(baseURL+"/application-load-balancer/:id", func(ctx echo.Context) error {
		return server.ApiUpdateLoadBalancerById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiUpdateLoadBalancerById"]...)
	e.DELETE(baseURL+"/application-load-balancer/:id", func(ctx echo.Context) error {
		return server.ApiDeleteLoadBalancerById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiDeleteLoadBalancerById"]...)
	e.POST(baseURL+"/vpn-gateway", server.ApiCreateVpnGateway, operationMiddlewares["apiCreateVpnGateway"]...)
	e.GET(baseURL+"/vpn-gateway", server.ApiGetVpnGateways, operationMiddlewares["apiGetVpnGateways"]...)
	e.GET(baseURL+"/vpn-gateway/:id", func(ctx echo.Context) error {
		return server.ApiGetVpnGatewayById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiGetVpnGatewayById"]...)
	e.PUT(baseURL+"/vpn-gateway/:id", func(ctx echo.Context) error {
		return server.ApiUpdateVpnGatewayById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiUpdateVpnGatewayById"]...)
	e.DELETE(baseURL+"/vpn-gateway/:id", func(ctx echo.Context) error {
		return server.ApiDeleteVpnGatewayById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiDeleteVpnGatewayById"]...)
	e.GET(baseURL+"/vpn-gateway/:id/cert", func(ctx echo.Context) error {
		return server.ApiGetVpnGatewayCertById(ctx, ctx.Param("id"))
	}, operationMiddlewares["apiGetVpnGatewayCertById"]...)
}
