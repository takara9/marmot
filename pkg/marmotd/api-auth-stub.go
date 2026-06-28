package marmotd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

func apiErrorJSON(ctx echo.Context, status int, message string) error {
	return ctx.JSON(status, api.Error{Code: int32(status), Message: message})
}

func mapAuthDBError(ctx echo.Context, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return apiErrorJSON(ctx, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrInvalidCredentials), errors.Is(err, db.ErrUserLocked), errors.Is(err, db.ErrUserDisabled):
		return apiErrorJSON(ctx, http.StatusUnauthorized, err.Error())
	case errors.Is(err, db.ErrUpdateConflict):
		return apiErrorJSON(ctx, http.StatusConflict, err.Error())
	default:
		return apiErrorJSON(ctx, http.StatusInternalServerError, err.Error())
	}
}

func bearerTokenFromAuthzHeader(authz string) (string, bool) {
	a := strings.TrimSpace(authz)
	if a == "" {
		return "", false
	}
	parts := strings.SplitN(a, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}

func validatePasswordPolicy(password string) error {
	p := strings.TrimSpace(password)
	if len(p) < 8 {
		return errors.New("newPassword must be at least 8 characters")
	}
	for _, ch := range p {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return errors.New("newPassword must be alphanumeric")
	}
	return nil
}

func (s *Server) requireBearerAuth(ctx echo.Context) (api.User, api.ApiKey, string, error) {
	token, ok := bearerTokenFromAuthzHeader(ctx.Request().Header.Get("Authorization"))
	if !ok {
		return api.User{}, api.ApiKey{}, "", apiErrorJSON(ctx, http.StatusUnauthorized, "missing or invalid Authorization header")
	}
	user, key, err := s.Ma.Db.AuthenticateApiKey(token)
	if err != nil {
		return api.User{}, api.ApiKey{}, "", mapAuthDBError(ctx, err)
	}
	return user, key, token, nil
}

func (s *Server) ApiAuthLogin(ctx echo.Context) error {
	var req api.AuthLoginRequest
	if err := ctx.Bind(&req); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	userID := strings.TrimSpace(req.UserId)
	password := strings.TrimSpace(req.Password)
	if userID == "" || password == "" {
		return apiErrorJSON(ctx, http.StatusBadRequest, "userId and password are required")
	}

	user, err := s.Ma.Db.AuthenticateUser(userID, password)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	apiKey, rawToken, err := s.Ma.Db.CreateUserApiKey(userID, api.ApiKeyCreateRequest{Comment: util.StringPtr("login-session")})
	if err != nil {
		return mapAuthDBError(ctx, err)
	}

	resp := api.AuthLoginResponse{
		AccessToken: rawToken,
		TokenType:   "Bearer",
		User:        &user,
	}
	if user.Spec.MustChangePassword != nil {
		resp.MustChangePassword = user.Spec.MustChangePassword
	}
	if apiKey.Spec.ExpiresAt != nil && apiKey.Spec.IssuedAt != nil {
		expiresIn := int64(apiKey.Spec.ExpiresAt.Sub(*apiKey.Spec.IssuedAt).Seconds())
		if expiresIn > 0 {
			resp.ExpiresIn = &expiresIn
		}
	}
	ctx.Response().Header().Set("X-Marmot-ApiKey-Id", apiKey.Metadata.Id)
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) ApiAuthLogout(ctx echo.Context) error {
	user, key, _, err := s.requireBearerAuth(ctx)
	if err != nil {
		return err
	}
	if err := s.Ma.Db.DeleteUserApiKey(user.Metadata.Id, key.Metadata.Id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.NoContent(http.StatusNoContent)
		}
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiAuthMe(ctx echo.Context) error {
	user, _, _, err := s.requireBearerAuth(ctx)
	if err != nil {
		return err
	}
	roles := []string{}
	if user.Spec.Roles != nil {
		roles = append(roles, (*user.Spec.Roles)...)
	}
	resp := api.AuthMe{UserId: user.Metadata.Id, Roles: roles}
	if name := strings.TrimSpace(user.Metadata.Name); name != "" {
		resp.DisplayName = &name
	}
	resp.Enabled = &user.Spec.Enabled
	if user.Spec.MustChangePassword != nil {
		resp.MustChangePassword = user.Spec.MustChangePassword
	}
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) ApiAuthzCheck(ctx echo.Context) error {
	user, _, _, err := s.requireBearerAuth(ctx)
	if err != nil {
		return err
	}
	var req api.AuthzCheckRequest
	if err := ctx.Bind(&req); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	resource := strings.TrimSpace(req.Resource)
	action := strings.TrimSpace(req.Action)
	if resource == "" || action == "" {
		return apiErrorJSON(ctx, http.StatusBadRequest, "resource and action are required")
	}
	checkUserID := user.Metadata.Id
	if req.UserId != nil && strings.TrimSpace(*req.UserId) != "" {
		checkUserID = strings.TrimSpace(*req.UserId)
	}
	allowed, err := s.Ma.Db.Authorize(checkUserID, resource, action)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	resp := api.AuthzCheckResponse{Allowed: allowed}
	if !allowed {
		reason := "access denied"
		resp.Reason = &reason
	}
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) ApiListRoles(ctx echo.Context) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	roles, err := s.Ma.Db.ListRoles()
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, roles)
}

func (s *Server) ApiGetRoleByName(ctx echo.Context, roleName string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	role, err := s.Ma.Db.GetRoleByName(roleName)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, role)
}

func (s *Server) ApiListUsers(ctx echo.Context) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	users, err := s.Ma.Db.ListUsers()
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, users)
}

func (s *Server) ApiCreateUser(ctx echo.Context) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	var input api.User
	if err := ctx.Bind(&input); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	created, err := s.Ma.Db.CreateUser(input)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return apiErrorJSON(ctx, http.StatusConflict, err.Error())
		}
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusCreated, created)
}

func (s *Server) ApiDeleteUserById(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	if err := s.Ma.Db.DeleteUserById(userId); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiGetUserById(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	user, err := s.Ma.Db.GetUserById(userId)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, user)
}

func (s *Server) ApiUpdateUserById(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	var input api.User
	if err := ctx.Bind(&input); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err := s.Ma.Db.UpdateUser(userId, input); err != nil {
		return mapAuthDBError(ctx, err)
	}
	user, err := s.Ma.Db.GetUserById(userId)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, user)
}

func (s *Server) ApiListUserApiKeys(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	keys, err := s.Ma.Db.ListUserApiKeys(userId)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, keys)
}

func (s *Server) ApiCreateUserApiKey(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	var req api.ApiKeyCreateRequest
	if err := ctx.Bind(&req); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	created, rawToken, err := s.Ma.Db.CreateUserApiKey(userId, req)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	ctx.Response().Header().Set("X-Marmot-Api-Key", rawToken)
	return ctx.JSON(http.StatusCreated, created)
}

func (s *Server) ApiDeleteUserApiKey(ctx echo.Context, userId string, apiKeyId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	if err := s.Ma.Db.DeleteUserApiKey(userId, apiKeyId); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiLockUserById(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	if err := s.Ma.Db.LockUserById(userId); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiChangeUserPassword(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	var req api.PasswordChangeRequest
	if err := ctx.Bind(&req); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err := validatePasswordPolicy(req.NewPassword); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, err.Error())
	}
	if req.CurrentPassword != nil && strings.TrimSpace(*req.CurrentPassword) != "" {
		if _, err := s.Ma.Db.AuthenticateUser(userId, *req.CurrentPassword); err != nil {
			return mapAuthDBError(ctx, err)
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apiErrorJSON(ctx, http.StatusInternalServerError, fmt.Sprintf("failed to hash password: %v", err))
	}

	mustChange := util.BoolPtr(false)
	if req.CurrentPassword == nil || strings.TrimSpace(*req.CurrentPassword) == "" {
		mustChange = util.BoolPtr(true)
	}
	if err := s.Ma.Db.SetUserPasswordHash(userId, string(hash), mustChange); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiListUserRoles(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	roles, err := s.Ma.Db.ListUserRoles(userId)
	if err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.JSON(http.StatusOK, roles)
}

func (s *Server) ApiAddUserRole(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	var req api.RoleAssignmentRequest
	if err := ctx.Bind(&req); err != nil {
		return apiErrorJSON(ctx, http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.RoleName) == "" {
		return apiErrorJSON(ctx, http.StatusBadRequest, "roleName is required")
	}
	if err := s.Ma.Db.AddUserRole(userId, req.RoleName); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiDeleteUserRole(ctx echo.Context, userId string, roleName string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	if err := s.Ma.Db.DeleteUserRole(userId, roleName); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) ApiUnlockUserById(ctx echo.Context, userId string) error {
	if _, _, _, err := s.requireBearerAuth(ctx); err != nil {
		return err
	}
	if err := s.Ma.Db.UnlockUserById(userId); err != nil {
		return mapAuthDBError(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}
