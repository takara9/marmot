package marmotd

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type operationRBACRule struct {
	Resource  string
	Verb      string
	AllowSelf bool
	SelfParam string
}

func rbacOperationMiddlewares(s *Server) map[string][]echo.MiddlewareFunc {
	rules := map[string]operationRBACRule{
		"apiAuthLogout": {Resource: "", Verb: ""},
		"apiAuthMe":     {Resource: "", Verb: ""},
		"apiAuthzCheck": {Resource: "", Verb: ""},

		"apiListRoles":    {Resource: "", Verb: ""},
		"apiGetRoleByName": {Resource: "", Verb: ""},

		"apiGetMarmotStatus":  {Resource: "Cluster", Verb: "read"},
		"apiGetMarmotCluster": {Resource: "Cluster", Verb: "read"},

		"apiGetServers":                    {Resource: "Server", Verb: "read"},
		"apiCreateServer":                  {Resource: "Server", Verb: "create"},
		"apiGetServerById":                 {Resource: "Server", Verb: "read"},
		"apiUpdateServerById":              {Resource: "Server", Verb: "update"},
		"apiDeleteServerById":              {Resource: "Server", Verb: "delete"},
		"apiStartServerById":               {Resource: "Server", Verb: "update"},
		"apiStopServerById":                {Resource: "Server", Verb: "update"},
		"apiMakeImageEntryFromRunningVMById": {Resource: "Server", Verb: "update"},
		"apiConsoleServerById":             {Resource: "Server", Verb: "read"},

		"apiListVolumes":      {Resource: "Volume", Verb: "read"},
		"apiCreateVolume":     {Resource: "Volume", Verb: "create"},
		"apiShowVolumeById":   {Resource: "Volume", Verb: "read"},
		"apiUpdateVolumeById": {Resource: "Volume", Verb: "update"},
		"apiDeleteVolumeById": {Resource: "Volume", Verb: "delete"},

		"apiGetNetworks":          {Resource: "Network", Verb: "read"},
		"apiCreateNetwork":        {Resource: "Network", Verb: "create"},
		"apiGetNetworkById":       {Resource: "Network", Verb: "read"},
		"apiUpdateNetworkById":    {Resource: "Network", Verb: "update"},
		"apiDeleteNetworkById":    {Resource: "Network", Verb: "delete"},
		"apiListIpNetworks":       {Resource: "Network", Verb: "read"},
		"apiGetIpAddressesByNetwork": {Resource: "Network", Verb: "read"},
		"apiGetNetworkIpNetworks": {Resource: "Network", Verb: "read"},

		"apiGetGateways":        {Resource: "ServerGateway", Verb: "read"},
		"apiCreateGateway":      {Resource: "ServerGateway", Verb: "create"},
		"apiGetGatewayById":     {Resource: "ServerGateway", Verb: "read"},
		"apiUpdateGatewayById":  {Resource: "ServerGateway", Verb: "update"},
		"apiDeleteGatewayById":  {Resource: "ServerGateway", Verb: "delete"},
		"apiGetGatewayCertById": {Resource: "ServerGateway", Verb: "read"},

		"apiGetVpnGateways":        {Resource: "VpnGateway", Verb: "read"},
		"apiCreateVpnGateway":      {Resource: "VpnGateway", Verb: "create"},
		"apiGetVpnGatewayById":     {Resource: "VpnGateway", Verb: "read"},
		"apiUpdateVpnGatewayById":  {Resource: "VpnGateway", Verb: "update"},
		"apiDeleteVpnGatewayById":  {Resource: "VpnGateway", Verb: "delete"},
		"apiGetVpnGatewayCertById": {Resource: "VpnGateway", Verb: "read"},

		"apiGetNetworkLoadBalancers":      {Resource: "NetworkLoadBalancer", Verb: "read"},
		"apiCreateNetworkLoadBalancer":    {Resource: "NetworkLoadBalancer", Verb: "create"},
		"apiGetNetworkLoadBalancerById":   {Resource: "NetworkLoadBalancer", Verb: "read"},
		"apiUpdateNetworkLoadBalancerById": {Resource: "NetworkLoadBalancer", Verb: "update"},
		"apiDeleteNetworkLoadBalancerById": {Resource: "NetworkLoadBalancer", Verb: "delete"},

		"apiGetLoadBalancers":      {Resource: "ApplicationLoadBalancer", Verb: "read"},
		"apiCreateLoadBalancer":    {Resource: "ApplicationLoadBalancer", Verb: "create"},
		"apiGetLoadBalancerById":   {Resource: "ApplicationLoadBalancer", Verb: "read"},
		"apiUpdateLoadBalancerById": {Resource: "ApplicationLoadBalancer", Verb: "update"},
		"apiDeleteLoadBalancerById": {Resource: "ApplicationLoadBalancer", Verb: "delete"},

		"apiGetImages":            {Resource: "Server", Verb: "read"},
		"apiCreateImage":          {Resource: "Server", Verb: "create"},
		"apiGetImageById":         {Resource: "Server", Verb: "read"},
		"apiUpdateImageById":      {Resource: "Server", Verb: "update"},
		"apiDeleteImageById":      {Resource: "Server", Verb: "delete"},
		"apiDownloadImageQcow2ById": {Resource: "Server", Verb: "read"},
		"apiImportImageArchive":   {Resource: "Server", Verb: "create"},

		"apiListUsers":       {Resource: "User", Verb: "read"},
		"apiCreateUser":      {Resource: "User", Verb: "create"},
		"apiGetUserById":     {Resource: "User", Verb: "read", AllowSelf: true, SelfParam: "userId"},
		"apiUpdateUserById":  {Resource: "User", Verb: "update"},
		"apiDeleteUserById":  {Resource: "User", Verb: "delete"},
		"apiLockUserById":    {Resource: "User", Verb: "update"},
		"apiUnlockUserById":  {Resource: "User", Verb: "update"},
		"apiChangeUserPassword": {Resource: "User", Verb: "update", AllowSelf: true, SelfParam: "userId"},
		"apiListUserRoles":      {Resource: "User", Verb: "read", AllowSelf: true, SelfParam: "userId"},
		"apiAddUserRole":        {Resource: "User", Verb: "update"},
		"apiDeleteUserRole":     {Resource: "User", Verb: "update"},
		"apiListUserApiKeys":    {Resource: "User", Verb: "read", AllowSelf: true, SelfParam: "userId"},
		"apiCreateUserApiKey":   {Resource: "User", Verb: "create", AllowSelf: true, SelfParam: "userId"},
		"apiDeleteUserApiKey":   {Resource: "User", Verb: "delete", AllowSelf: true, SelfParam: "userId"},
	}

	middlewares := make(map[string][]echo.MiddlewareFunc, len(rules))
	for operationID, rule := range rules {
		r := rule
		middlewares[operationID] = []echo.MiddlewareFunc{func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(ctx echo.Context) error {
				user, _, _, err := s.requireBearerAuth(ctx)
				if err != nil {
					return err
				}
				if ctx.Response().Committed {
					// requireBearerAuth already wrote an error response (e.g. 401)
					return nil
				}

				if strings.TrimSpace(r.Resource) == "" || strings.TrimSpace(r.Verb) == "" {
					return next(ctx)
				}

				if r.AllowSelf && strings.TrimSpace(r.SelfParam) != "" {
					if strings.TrimSpace(ctx.Param(r.SelfParam)) == strings.TrimSpace(user.Metadata.Id) {
						return next(ctx)
					}
				}

				allowed, authErr := s.Ma.Db.Authorize(user.Metadata.Id, r.Resource, r.Verb)
				if authErr != nil {
					return mapAuthDBError(ctx, authErr)
				}
				if !allowed {
					return apiErrorJSON(ctx, http.StatusForbidden, "forbidden")
				}

				return next(ctx)
			}
		}}
	}

	return middlewares
}
