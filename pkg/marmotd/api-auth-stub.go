package marmotd

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

func notImplemented(ctx echo.Context, endpoint string) error {
	return ctx.JSON(http.StatusNotImplemented, api.Error{
		Code:    http.StatusNotImplemented,
		Message: endpoint + " is not implemented yet",
	})
}

func (s *Server) ApiAuthLogin(ctx echo.Context) error {
	return notImplemented(ctx, "ApiAuthLogin")
}

func (s *Server) ApiAuthLogout(ctx echo.Context) error {
	return notImplemented(ctx, "ApiAuthLogout")
}

func (s *Server) ApiAuthMe(ctx echo.Context) error {
	return notImplemented(ctx, "ApiAuthMe")
}

func (s *Server) ApiAuthzCheck(ctx echo.Context) error {
	return notImplemented(ctx, "ApiAuthzCheck")
}

func (s *Server) ApiListRoles(ctx echo.Context) error {
	return notImplemented(ctx, "ApiListRoles")
}

func (s *Server) ApiGetRoleByName(ctx echo.Context, roleName string) error {
	_ = roleName
	return notImplemented(ctx, "ApiGetRoleByName")
}

func (s *Server) ApiListUsers(ctx echo.Context) error {
	return notImplemented(ctx, "ApiListUsers")
}

func (s *Server) ApiCreateUser(ctx echo.Context) error {
	return notImplemented(ctx, "ApiCreateUser")
}

func (s *Server) ApiDeleteUserById(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiDeleteUserById")
}

func (s *Server) ApiGetUserById(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiGetUserById")
}

func (s *Server) ApiUpdateUserById(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiUpdateUserById")
}

func (s *Server) ApiListUserApiKeys(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiListUserApiKeys")
}

func (s *Server) ApiCreateUserApiKey(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiCreateUserApiKey")
}

func (s *Server) ApiDeleteUserApiKey(ctx echo.Context, userId string, apiKeyId string) error {
	_ = userId
	_ = apiKeyId
	return notImplemented(ctx, "ApiDeleteUserApiKey")
}

func (s *Server) ApiLockUserById(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiLockUserById")
}

func (s *Server) ApiChangeUserPassword(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiChangeUserPassword")
}

func (s *Server) ApiListUserRoles(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiListUserRoles")
}

func (s *Server) ApiAddUserRole(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiAddUserRole")
}

func (s *Server) ApiDeleteUserRole(ctx echo.Context, userId string, roleName string) error {
	_ = userId
	_ = roleName
	return notImplemented(ctx, "ApiDeleteUserRole")
}

func (s *Server) ApiUnlockUserById(ctx echo.Context, userId string) error {
	_ = userId
	return notImplemented(ctx, "ApiUnlockUserById")
}