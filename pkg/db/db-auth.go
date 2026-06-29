package db

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

const (
	BootstrapAdminUserID       = "admin"
	BootstrapAdminPassword     = "passw0rd"
	BootstrapAdminRoleName     = "Administrator"
	BootstrapAdminComment      = "bootstrap administrator"
)

const (
	AuthUserPrefix          = "/marmot/user"
	AuthRolePrefix          = "/marmot/role"
	AuthUserApiKeyPrefix    = "/marmot/user-apikey"
	AuthApiKeyIndexPrefix   = "/marmot/apikey-index"
	AuthApiKeyTokenPrefixLen = 8
	AuthSessionIdleTimeout  = 30 * time.Minute
	ApiKeySessionTypeLogin  = "login"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserLocked = errors.New("user is locked")
var ErrUserDisabled = errors.New("user is disabled")

var builtinRoleNames = []string{
	"Administrator",
	"Network-Administrator",
	"Compute-Operator",
	"Viewer",
}

func builtinRoles() map[string]api.Role {
	return map[string]api.Role{
		"Administrator": roleTemplate("Administrator", "Full administrative access", []api.Permission{
			permission("Server", "create", "read", "update", "delete"),
			permission("Cluster", "create", "read", "update", "delete"),
			permission("Volume", "create", "read", "update", "delete"),
			permission("Network", "create", "read", "update", "delete"),
			permission("ServerGateway", "create", "read", "update", "delete"),
			permission("VpnGateway", "create", "read", "update", "delete"),
			permission("NetworkLoadBalancer", "create", "read", "update", "delete"),
			permission("ApplicationLoadBalancer", "create", "read", "update", "delete"),
			permission("User", "create", "read", "update", "delete"),
		}),
		"Network-Administrator": roleTemplate("Network-Administrator", "Network administration access", []api.Permission{
			permission("Server", "read"),
			permission("Cluster", "read"),
			permission("Volume", "read"),
			permission("Network", "create", "read", "update", "delete"),
			permission("ServerGateway", "read"),
			permission("VpnGateway", "create", "read", "update", "delete"),
			permission("NetworkLoadBalancer", "create", "read", "update", "delete"),
			permission("ApplicationLoadBalancer", "create", "read", "update", "delete"),
			permission("User", "read"),
		}),
		"Compute-Operator": roleTemplate("Compute-Operator", "Compute operations access", []api.Permission{
			permission("Server", "create", "read", "update", "delete"),
			permission("Cluster", "read"),
			permission("Volume", "create", "read", "update"),
			permission("Network", "read"),
			permission("ServerGateway", "create", "read", "update", "delete"),
			permission("VpnGateway", "read"),
			permission("NetworkLoadBalancer", "read"),
			permission("ApplicationLoadBalancer", "read"),
			permission("User", "read"),
		}),
		"Viewer": roleTemplate("Viewer", "Read-only access", []api.Permission{
			permission("Server", "read"),
			permission("Cluster", "read"),
			permission("Volume", "read"),
			permission("Network", "read"),
			permission("ServerGateway", "read"),
			permission("VpnGateway", "read"),
			permission("NetworkLoadBalancer", "read"),
			permission("ApplicationLoadBalancer", "read"),
			permission("User", "read"),
		}),
	}
}

func permission(resource string, verbs ...string) api.Permission {
	return api.Permission{Resource: resource, Verbs: verbs}
}

func roleTemplate(name, description string, permissions []api.Permission) api.Role {
	return api.Role{
		ApiVersion: "v1",
		Kind:       "Role",
		Metadata: api.Metadata{
			Id:   name,
			Name: name,
		},
		Spec: api.RoleSpec{
			Description: util.StringPtr(description),
			Permissions: permissions,
		},
	}
}

func userKey(userID string) string {
	return AuthUserPrefix + "/" + strings.TrimSpace(userID)
}

func userApiKeyKey(userID, apiKeyID string) string {
	return AuthUserApiKeyPrefix + "/" + strings.TrimSpace(userID) + "/" + strings.TrimSpace(apiKeyID)
}

func apiKeyIndexKey(token string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return AuthApiKeyIndexPrefix + "/" + hex.EncodeToString(hash[:])
}

func apiKeyIdIndexKey(userID, apiKeyID string) string {
	return AuthApiKeyIndexPrefix + "/by-id/" + strings.TrimSpace(userID) + "/" + strings.TrimSpace(apiKeyID)
}

func roleKey(roleName string) string {
	return AuthRolePrefix + "/" + strings.TrimSpace(roleName)
}

func normalizeUserIdentity(user *api.User, fallbackID string) {
	if user == nil {
		return
	}
	fallbackID = strings.TrimSpace(fallbackID)
	if strings.TrimSpace(user.ApiVersion) == "" {
		user.ApiVersion = "v1"
	}
	if strings.TrimSpace(user.Kind) == "" {
		user.Kind = "User"
	}
	if strings.TrimSpace(user.Metadata.Id) == "" {
		user.Metadata.Id = fallbackID
	}
	if strings.TrimSpace(user.Metadata.Name) == "" {
		user.Metadata.Name = user.Metadata.Id
	}
	if user.Metadata.Key == nil || strings.TrimSpace(*user.Metadata.Key) == "" {
		key := userKey(user.Metadata.Id)
		user.Metadata.Key = &key
	}
	if user.Spec.Roles == nil {
		roles := []string{}
		user.Spec.Roles = &roles
	}
	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
}

func normalizeRoleIdentity(role *api.Role, fallbackName string) {
	if role == nil {
		return
	}
	fallbackName = strings.TrimSpace(fallbackName)
	if strings.TrimSpace(role.ApiVersion) == "" {
		role.ApiVersion = "v1"
	}
	if strings.TrimSpace(role.Kind) == "" {
		role.Kind = "Role"
	}
	if strings.TrimSpace(role.Metadata.Id) == "" {
		role.Metadata.Id = fallbackName
	}
	if strings.TrimSpace(role.Metadata.Name) == "" {
		role.Metadata.Name = role.Metadata.Id
	}
	if role.Metadata.Key == nil || strings.TrimSpace(*role.Metadata.Key) == "" {
		key := roleKey(role.Metadata.Name)
		role.Metadata.Key = &key
	}
}

func normalizeApiKey(apiKey *api.ApiKey, fallbackUserID, fallbackKeyID string) {
	if apiKey == nil {
		return
	}
	if strings.TrimSpace(apiKey.ApiVersion) == "" {
		apiKey.ApiVersion = "v1"
	}
	if strings.TrimSpace(apiKey.Kind) == "" {
		apiKey.Kind = "ApiKey"
	}
	if strings.TrimSpace(apiKey.Metadata.Id) == "" {
		apiKey.Metadata.Id = strings.TrimSpace(fallbackKeyID)
	}
	if strings.TrimSpace(apiKey.Metadata.Name) == "" {
		apiKey.Metadata.Name = apiKey.Metadata.Id
	}
	if apiKey.Metadata.Key == nil || strings.TrimSpace(*apiKey.Metadata.Key) == "" {
		key := userApiKeyKey(fallbackUserID, apiKey.Metadata.Id)
		apiKey.Metadata.Key = &key
	}
	if apiKey.Spec.Revoked == nil {
		apiKey.Spec.Revoked = util.BoolPtr(false)
	}
	if apiKey.Spec.SessionType != nil {
		t := strings.TrimSpace(*apiKey.Spec.SessionType)
		apiKey.Spec.SessionType = &t
	}
	if apiKey.Status == nil {
		apiKey.Status = &api.ApiKeyStatus{}
	}
}

func isLoginSessionApiKey(apiKey api.ApiKey) bool {
	if apiKey.Spec.SessionType != nil {
		return strings.EqualFold(strings.TrimSpace(*apiKey.Spec.SessionType), ApiKeySessionTypeLogin)
	}
	if apiKey.Spec.Comment != nil {
		return strings.TrimSpace(*apiKey.Spec.Comment) == "login-session"
	}
	return false
}

func isUserLocked(user api.User) bool {
	if user.Status == nil {
		return false
	}
	if user.Status.LockedAt != nil {
		return true
	}
	if user.Status.LockedUntil != nil && time.Now().Before(*user.Status.LockedUntil) {
		return true
	}
	return false
}

func userHasPermission(user api.User, resource, verb string, roles map[string]api.Role) bool {
	if user.Spec.Roles == nil {
		return false
	}
	for _, roleName := range *user.Spec.Roles {
		role, ok := roles[strings.TrimSpace(roleName)]
		if !ok {
			continue
		}
		if roleAllows(role, resource, verb) {
			return true
		}
	}
	return false
}

func roleAllows(role api.Role, resource, verb string) bool {
	for _, perm := range role.Spec.Permissions {
		if !strings.EqualFold(strings.TrimSpace(perm.Resource), strings.TrimSpace(resource)) {
			continue
		}
		for _, candidate := range perm.Verbs {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(verb)) {
				return true
			}
		}
	}
	return false
}

// CreateUser stores a new user entry.
func (d *Database) CreateUser(input api.User) (api.User, error) {
	user, err := util.DeepCopy(input)
	if err != nil {
		return api.User{}, fmt.Errorf("failed to copy user: %w", err)
	}

	if strings.TrimSpace(user.Metadata.Id) == "" {
		if strings.TrimSpace(user.Metadata.Name) != "" {
			user.Metadata.Id = strings.TrimSpace(user.Metadata.Name)
		} else {
			user.Metadata.Id = uuid.NewString()
		}
	}
	normalizeUserIdentity(&user, user.Metadata.Id)

	key := userKey(user.Metadata.Id)
	mutex, err := d.LockKey(key)
	if err != nil {
		return api.User{}, err
	}
	defer d.UnlockKey(mutex)

	if _, err := d.GetJSON(key, &api.User{}); err == nil {
		return api.User{}, fmt.Errorf("user already exists: %s", user.Metadata.Id)
	} else if err != ErrNotFound {
		return api.User{}, err
	}

	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
	if user.Status.PasswordUpdatedAt == nil && user.Spec.PasswordHash != nil && strings.TrimSpace(*user.Spec.PasswordHash) != "" {
		user.Status.PasswordUpdatedAt = util.TimePtr(time.Now())
	}
	if err := d.PutJSON(key, user); err != nil {
		return api.User{}, err
	}
	return user, nil
}

// EnsureBootstrapAdmin seeds the default admin user when the auth store is empty.
// Existing users are never modified.
func (d *Database) EnsureBootstrapAdmin() error {
	users, err := d.ListUsers()
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(BootstrapAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to generate bootstrap admin password hash: %w", err)
	}

	_, err = d.CreateUser(api.User{
		ApiVersion: "v1",
		Kind:       "User",
		Metadata: api.Metadata{
			Id:   BootstrapAdminUserID,
			Name: BootstrapAdminUserID,
		},
		Spec: api.UserSpec{
			Enabled:            true,
			Comment:            util.StringPtr(BootstrapAdminComment),
			PasswordHash:       util.StringPtr(string(hash)),
			Roles:              &[]string{BootstrapAdminRoleName},
			MustChangePassword: util.BoolPtr(true),
		},
	})
	if err != nil {
		return err
	}
	return nil
}

// GetUserById returns a user by id.
func (d *Database) GetUserById(userID string) (api.User, error) {
	var user api.User
	key := userKey(userID)
	if _, err := d.GetJSON(key, &user); err != nil {
		return api.User{}, err
	}
	normalizeUserIdentity(&user, userID)
	return user, nil
}

// ListUsers returns all users.
func (d *Database) ListUsers() (api.Users, error) {
	resp, err := d.GetByPrefix(AuthUserPrefix + "/")
	if err == ErrNotFound {
		return api.Users{}, nil
	}
	if err != nil {
		return nil, err
	}

	users := make([]api.User, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var user api.User
		if err := json.Unmarshal(kv.Value, &user); err != nil {
			slog.Error("ListUsers() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		userID := strings.TrimPrefix(string(kv.Key), AuthUserPrefix+"/")
		if idx := strings.Index(userID, "/"); idx >= 0 {
			userID = userID[:idx]
		}
		normalizeUserIdentity(&user, userID)
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Metadata.Id < users[j].Metadata.Id })
	return users, nil
}

// UpdateUser updates the stored user.
func (d *Database) UpdateUser(userID string, input api.User) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var current api.User
	resp, err := d.GetJSON(key, &current)
	if err != nil {
		return err
	}
	_ = resp
	util.PatchStruct(&current, input)
	current.Metadata.Id = strings.TrimSpace(userID)
	if strings.TrimSpace(current.Metadata.Name) == "" {
		current.Metadata.Name = current.Metadata.Id
	}
	normalizeUserIdentity(&current, userID)
	current.Status = current.Status
	if err := d.PutJSONCAS(key, resp.Kvs[0].ModRevision, current); err != nil {
		return err
	}
	return nil
}

// DeleteUserById deletes a user and all of the user's API keys.
func (d *Database) DeleteUserById(userID string) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	keys, err := d.listAllUserApiKeysLocked(userID)
	if err != nil && err != ErrNotFound {
		return err
	}
	for _, apiKey := range keys {
		if err := d.deleteUserApiKeyLocked(userID, apiKey.Metadata.Id); err != nil && err != ErrNotFound {
			return err
		}
	}
	return d.DeleteJSON(key)
}

// SetUserPasswordHash updates the password hash and password timestamp.
func (d *Database) SetUserPasswordHash(userID, passwordHash string, mustChangePassword *bool) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
	if user.Spec.PasswordHash == nil {
		user.Spec.PasswordHash = util.StringPtr(passwordHash)
	} else {
		*user.Spec.PasswordHash = passwordHash
	}
	if mustChangePassword != nil {
		user.Spec.MustChangePassword = mustChangePassword
	}
	user.Status.PasswordUpdatedAt = util.TimePtr(time.Now())
	if err := d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user); err != nil {
		return err
	}
	return nil
}

// LockUserById locks a user and invalidates all API keys.
func (d *Database) LockUserById(userID string) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
	user.Status.LockedAt = util.TimePtr(time.Now())
	user.Status.LockedUntil = nil
	user.Spec.Enabled = false

	keys, err := d.listAllUserApiKeysLocked(userID)
	if err != nil && err != ErrNotFound {
		return err
	}
	for _, apiKey := range keys {
		if err := d.revokeUserApiKeyLocked(userID, apiKey.Metadata.Id); err != nil && err != ErrNotFound {
			return err
		}
	}

	user.Status.ApiKeyCount = util.Int64PtrInt32(0)
	if err := d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user); err != nil {
		return err
	}
	return nil
}

// UnlockUserById unlocks a user.
func (d *Database) UnlockUserById(userID string) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
	user.Status.LockedAt = nil
	user.Status.LockedUntil = nil
	user.Spec.Enabled = true
	if err := d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user); err != nil {
		return err
	}
	return nil
}

// AuthenticateUser validates a password and returns the user on success.
func (d *Database) AuthenticateUser(userID, password string) (api.User, error) {
	user, err := d.GetUserById(userID)
	if err != nil {
		return api.User{}, err
	}
	if isUserLocked(user) {
		return api.User{}, ErrUserLocked
	}
	if !user.Spec.Enabled {
		return api.User{}, ErrUserDisabled
	}
	if user.Spec.PasswordHash == nil || strings.TrimSpace(*user.Spec.PasswordHash) == "" {
		return api.User{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.Spec.PasswordHash), []byte(password)); err != nil {
		return api.User{}, ErrInvalidCredentials
	}
	if err := d.recordUserLogin(userID); err != nil {
		slog.Warn("AuthenticateUser() login stamp update failed", "err", err, "userId", userID)
	}
	return d.GetUserById(userID)
}

func (d *Database) recordUserLogin(userID string) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
	user.Status.LastLoginAt = util.TimePtr(time.Now())
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user)
}

// ListRoles returns the built-in role catalog.
func (d *Database) ListRoles() (api.Roles, error) {
	roles := make(api.Roles, 0, len(builtinRoleNames))
	templates := builtinRoles()
	for _, name := range builtinRoleNames {
		role, ok := templates[name]
		if !ok {
			continue
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// GetRoleByName returns a built-in role by name.
func (d *Database) GetRoleByName(roleName string) (api.Role, error) {
	role, ok := builtinRoles()[strings.TrimSpace(roleName)]
	if !ok {
		return api.Role{}, ErrNotFound
	}
	normalizeRoleIdentity(&role, roleName)
	return role, nil
}

// ListUserRoles returns the roles assigned to a user.
func (d *Database) ListUserRoles(userID string) (api.RoleNames, error) {
	user, err := d.GetUserById(userID)
	if err != nil {
		return nil, err
	}
	if user.Spec.Roles == nil {
		return api.RoleNames{}, nil
	}
	roles := make(api.RoleNames, 0, len(*user.Spec.Roles))
	for _, roleName := range *user.Spec.Roles {
		trimmed := strings.TrimSpace(roleName)
		if trimmed == "" {
			continue
		}
		roles = append(roles, trimmed)
	}
	sort.Strings(roles)
	return roles, nil
}

// AddUserRole assigns a role to a user.
func (d *Database) AddUserRole(userID, roleName string) error {
	if _, err := d.GetRoleByName(roleName); err != nil {
		return err
	}
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	if user.Spec.Roles == nil {
		roles := []string{}
		user.Spec.Roles = &roles
	}
	trimmed := strings.TrimSpace(roleName)
	for _, current := range *user.Spec.Roles {
		if strings.EqualFold(strings.TrimSpace(current), trimmed) {
			return nil
		}
	}
	*user.Spec.Roles = append(*user.Spec.Roles, trimmed)
	sort.Strings(*user.Spec.Roles)
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user)
}

// DeleteUserRole removes a role from a user.
func (d *Database) DeleteUserRole(userID, roleName string) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)

	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	if user.Spec.Roles == nil || len(*user.Spec.Roles) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(roleName)
	filtered := make([]string, 0, len(*user.Spec.Roles))
	for _, current := range *user.Spec.Roles {
		if strings.EqualFold(strings.TrimSpace(current), trimmed) {
			continue
		}
		filtered = append(filtered, strings.TrimSpace(current))
	}
	user.Spec.Roles = &filtered
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user)
}

// Authorize checks whether a user may perform a verb on a resource.
func (d *Database) Authorize(userID, resource, verb string) (bool, error) {
	user, err := d.GetUserById(userID)
	if err != nil {
		return false, err
	}
	if !user.Spec.Enabled || isUserLocked(user) {
		return false, nil
	}
	if userHasPermission(user, resource, verb, builtinRoles()) {
		return true, nil
	}
	return false, nil
}

// CreateUserApiKey creates a new API key for a user and returns the raw token.
func (d *Database) CreateUserApiKey(userID string, req api.ApiKeyCreateRequest) (api.ApiKey, string, error) {
	if _, err := d.GetUserById(userID); err != nil {
		return api.ApiKey{}, "", err
	}
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return api.ApiKey{}, "", err
	}
	defer d.UnlockKey(mutex)

	user, err := d.GetUserById(userID)
	if err != nil {
		return api.ApiKey{}, "", err
	}
	if !user.Spec.Enabled || isUserLocked(user) {
		return api.ApiKey{}, "", ErrUserLocked
	}

	rawToken := uuid.NewString() + uuid.NewString()
	apiKeyID := uuid.NewString()
	tokenPrefix := rawToken[:AuthApiKeyTokenPrefixLen]
	apiKey := api.ApiKey{
		ApiVersion: "v1",
		Kind:       "ApiKey",
		Metadata: api.Metadata{
			Id:   apiKeyID,
			Name: apiKeyID,
		},
		Spec: api.ApiKeySpec{
			Comment:     req.Comment,
			ExpiresAt:   req.ExpiresAt,
			IssuedAt:    util.TimePtr(time.Now()),
			Revoked:     util.BoolPtr(false),
			SessionType: req.SessionType,
			TokenPrefix: &tokenPrefix,
		},
	}
	normalizeApiKey(&apiKey, userID, apiKeyID)

	if err := d.putUserApiKeyWithRetry(userID, apiKey, rawToken); err != nil {
		return api.ApiKey{}, "", err
	}
	if err := d.refreshUserApiKeyCountLocked(userID); err != nil {
		slog.Warn("CreateUserApiKey() count refresh failed", "err", err, "userId", userID)
	}
	return apiKey, rawToken, nil
}

func (d *Database) putUserApiKeyWithRetry(userID string, apiKey api.ApiKey, rawToken string) error {
	for {
		key := userApiKeyKey(userID, apiKey.Metadata.Id)
		if _, err := d.GetJSON(key, &api.ApiKey{}); err == nil {
			apiKey.Metadata.Id = uuid.NewString()
			apiKey.Metadata.Name = apiKey.Metadata.Id
			normalizeApiKey(&apiKey, userID, apiKey.Metadata.Id)
			continue
		} else if err != ErrNotFound {
			return err
		}
		if err := d.PutJSON(key, apiKey); err != nil {
			return err
		}
		indexKey := apiKeyIndexKey(rawToken)
		if err := d.PutJSON(indexKey, map[string]string{"userId": userID, "apiKeyId": apiKey.Metadata.Id}); err != nil {
			return err
		}
		idIndexKey := apiKeyIdIndexKey(userID, apiKey.Metadata.Id)
		if err := d.PutJSON(idIndexKey, map[string]string{"tokenHash": strings.TrimPrefix(indexKey, AuthApiKeyIndexPrefix+"/")}); err != nil {
			return err
		}
		return nil
	}
}

// ListUserApiKeys returns active API keys for a user.
func (d *Database) ListUserApiKeys(userID string) (api.ApiKeys, error) {
	keys, err := d.listAllUserApiKeys(userID)
	if err != nil {
		return nil, err
	}
	active := make(api.ApiKeys, 0, len(keys))
	for _, key := range keys {
		if key.Spec.Revoked != nil && *key.Spec.Revoked {
			continue
		}
		if key.Status != nil && key.Status.RevokedAt != nil {
			continue
		}
		active = append(active, key)
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Metadata.Id < active[j].Metadata.Id })
	return active, nil
}

func (d *Database) listAllUserApiKeys(userID string) ([]api.ApiKey, error) {
	resp, err := d.GetByPrefix(AuthUserApiKeyPrefix + "/" + strings.TrimSpace(userID) + "/")
	if err == ErrNotFound {
		return []api.ApiKey{}, nil
	}
	if err != nil {
		return nil, err
	}
	keys := make([]api.ApiKey, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var apiKey api.ApiKey
		if err := json.Unmarshal(kv.Value, &apiKey); err != nil {
			slog.Error("listAllUserApiKeys() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		parts := strings.Split(strings.TrimPrefix(string(kv.Key), AuthUserApiKeyPrefix+"/"), "/")
		keyID := ""
		if len(parts) >= 2 {
			keyID = parts[1]
		}
		normalizeApiKey(&apiKey, userID, keyID)
		keys = append(keys, apiKey)
	}
	return keys, nil
}

func (d *Database) listAllUserApiKeysLocked(userID string) ([]api.ApiKey, error) {
	return d.listAllUserApiKeys(userID)
}

// DeleteUserApiKey revokes and removes a user's API key.
func (d *Database) DeleteUserApiKey(userID, apiKeyID string) error {
	key := userKey(userID)
	mutex, err := d.LockKey(key)
	if err != nil {
		return err
	}
	defer d.UnlockKey(mutex)
	return d.deleteUserApiKeyLocked(userID, apiKeyID)
}

func (d *Database) deleteUserApiKeyLocked(userID, apiKeyID string) error {
	key := userApiKeyKey(userID, apiKeyID)
	var apiKey api.ApiKey
	if _, err := d.GetJSON(key, &apiKey); err != nil {
		return err
	}
	if err := d.revokeUserApiKeyLocked(userID, apiKeyID); err != nil {
		return err
	}
	return d.refreshUserApiKeyCountLocked(userID)
}

func (d *Database) physicallyDeleteUserApiKeyLocked(userID, apiKeyID string) error {
	var indexRef struct {
		TokenHash string `json:"tokenHash"`
	}
	if _, err := d.GetJSON(apiKeyIdIndexKey(userID, apiKeyID), &indexRef); err == nil {
		if strings.TrimSpace(indexRef.TokenHash) != "" {
			_ = d.DeleteJSON(AuthApiKeyIndexPrefix + "/" + strings.TrimSpace(indexRef.TokenHash))
		}
		_ = d.DeleteJSON(apiKeyIdIndexKey(userID, apiKeyID))
	} else if err != ErrNotFound {
		return err
	}
	return d.DeleteJSON(userApiKeyKey(userID, apiKeyID))
}

// CleanupRevokedApiKeysOlderThan physically deletes revoked API keys whose RevokedAt is older than the threshold.
func (d *Database) CleanupRevokedApiKeysOlderThan(olderThan time.Duration) (int, error) {
	if olderThan < 0 {
		olderThan = 0
	}
	resp, err := d.GetByPrefix(AuthUserApiKeyPrefix + "/")
	if err == ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	type cleanupTarget struct {
		UserID   string
		ApiKeyID string
	}

	cutoff := time.Now().Add(-olderThan)
	targets := make([]cleanupTarget, 0)

	for _, kv := range resp.Kvs {
		path := strings.TrimPrefix(string(kv.Key), AuthUserApiKeyPrefix+"/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			continue
		}
		userID := strings.TrimSpace(parts[0])
		apiKeyID := strings.TrimSpace(parts[1])
		if userID == "" || apiKeyID == "" {
			continue
		}

		var rec api.ApiKey
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Warn("CleanupRevokedApiKeysOlderThan() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		normalizeApiKey(&rec, userID, apiKeyID)

		isRevoked := (rec.Spec.Revoked != nil && *rec.Spec.Revoked) || (rec.Status != nil && rec.Status.RevokedAt != nil)
		if !isRevoked {
			continue
		}
		if rec.Status == nil || rec.Status.RevokedAt == nil {
			continue
		}
		if rec.Status.RevokedAt.After(cutoff) {
			continue
		}

		targets = append(targets, cleanupTarget{UserID: userID, ApiKeyID: apiKeyID})
	}

	deletedCount := 0
	var firstErr error
	for _, target := range targets {
		mutex, err := d.LockKey(userKey(target.UserID))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("CleanupRevokedApiKeysOlderThan() lock failed", "err", err, "userId", target.UserID)
			continue
		}

		delErr := d.physicallyDeleteUserApiKeyLocked(target.UserID, target.ApiKeyID)
		d.UnlockKey(mutex)

		if delErr != nil {
			if delErr == ErrNotFound {
				continue
			}
			if firstErr == nil {
				firstErr = delErr
			}
			slog.Warn("CleanupRevokedApiKeysOlderThan() delete failed", "err", delErr, "userId", target.UserID, "apiKeyId", target.ApiKeyID)
			continue
		}
		deletedCount++
	}

	return deletedCount, firstErr
}

// RevokeIdleLoginSessionsOlderThan revokes login session API keys whose last activity is older than the threshold.
func (d *Database) RevokeIdleLoginSessionsOlderThan(olderThan time.Duration) (int, error) {
	if olderThan < 0 {
		olderThan = 0
	}
	resp, err := d.GetByPrefix(AuthUserApiKeyPrefix + "/")
	if err == ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	type revokeTarget struct {
		UserID   string
		ApiKeyID string
	}

	cutoff := time.Now().Add(-olderThan)
	targetsByUser := make(map[string][]string)

	for _, kv := range resp.Kvs {
		path := strings.TrimPrefix(string(kv.Key), AuthUserApiKeyPrefix+"/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			continue
		}
		userID := strings.TrimSpace(parts[0])
		apiKeyID := strings.TrimSpace(parts[1])
		if userID == "" || apiKeyID == "" {
			continue
		}

		var rec api.ApiKey
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			slog.Warn("RevokeIdleLoginSessionsOlderThan() unmarshal failed", "err", err, "key", string(kv.Key))
			continue
		}
		normalizeApiKey(&rec, userID, apiKeyID)

		if !isLoginSessionApiKey(rec) {
			continue
		}
		if rec.Spec.Revoked != nil && *rec.Spec.Revoked {
			continue
		}
		if rec.Status != nil && rec.Status.RevokedAt != nil {
			continue
		}

		lastActivityAt := time.Time{}
		if rec.Status != nil && rec.Status.LastUsedAt != nil {
			lastActivityAt = *rec.Status.LastUsedAt
		} else if rec.Spec.IssuedAt != nil {
			lastActivityAt = *rec.Spec.IssuedAt
		}
		if lastActivityAt.IsZero() || lastActivityAt.After(cutoff) {
			continue
		}

		targetsByUser[userID] = append(targetsByUser[userID], apiKeyID)
	}

	revokedCount := 0
	var firstErr error
	for userID, keyIDs := range targetsByUser {
		mutex, err := d.LockKey(userKey(userID))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("RevokeIdleLoginSessionsOlderThan() lock failed", "err", err, "userId", userID)
			continue
		}

		for _, keyID := range keyIDs {
			if err := d.revokeUserApiKeyLocked(userID, keyID); err != nil {
				if err == ErrNotFound {
					continue
				}
				if firstErr == nil {
					firstErr = err
				}
				slog.Warn("RevokeIdleLoginSessionsOlderThan() revoke failed", "err", err, "userId", userID, "apiKeyId", keyID)
				continue
			}
			revokedCount++
		}

		if err := d.refreshUserApiKeyCountLocked(userID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("RevokeIdleLoginSessionsOlderThan() refresh count failed", "err", err, "userId", userID)
		}

		d.UnlockKey(mutex)
	}

	return revokedCount, firstErr
}

func (d *Database) revokeUserApiKeyLocked(userID, apiKeyID string) error {
	key := userApiKeyKey(userID, apiKeyID)
	var apiKey api.ApiKey
	resp, err := d.GetJSON(key, &apiKey)
	if err != nil {
		return err
	}
	if apiKey.Spec.Revoked == nil {
		apiKey.Spec.Revoked = util.BoolPtr(true)
	} else {
		*apiKey.Spec.Revoked = true
	}
	if apiKey.Status == nil {
		apiKey.Status = &api.ApiKeyStatus{}
	}
	apiKey.Status.RevokedAt = util.TimePtr(time.Now())
	if err := d.PutJSONCAS(key, resp.Kvs[0].ModRevision, apiKey); err != nil {
		return err
	}
	var indexRef struct {
		TokenHash string `json:"tokenHash"`
	}
	if _, err := d.GetJSON(apiKeyIdIndexKey(userID, apiKeyID), &indexRef); err == nil {
		if strings.TrimSpace(indexRef.TokenHash) != "" {
			_ = d.DeleteJSON(AuthApiKeyIndexPrefix + "/" + strings.TrimSpace(indexRef.TokenHash))
		}
		_ = d.DeleteJSON(apiKeyIdIndexKey(userID, apiKeyID))
	}
	return nil
}

func (d *Database) refreshUserApiKeyCountLocked(userID string) error {
	key := userKey(userID)
	var user api.User
	resp, err := d.GetJSON(key, &user)
	if err != nil {
		return err
	}
	keys, err := d.listAllUserApiKeys(userID)
	if err != nil && err != ErrNotFound {
		return err
	}
	count := int32(0)
	for _, apiKey := range keys {
		if apiKey.Spec.Revoked != nil && *apiKey.Spec.Revoked {
			continue
		}
		count++
	}
	if user.Status == nil {
		user.Status = &api.UserStatus{}
	}
	user.Status.ApiKeyCount = &count
	return d.PutJSONCAS(key, resp.Kvs[0].ModRevision, user)
}

// AuthenticateApiKey resolves a bearer token to its user and API key.
func (d *Database) AuthenticateApiKey(token string) (api.User, api.ApiKey, error) {
	indexKey := apiKeyIndexKey(token)
	var ref struct {
		UserID   string `json:"userId"`
		ApiKeyID string `json:"apiKeyId"`
	}
	if _, err := d.GetJSON(indexKey, &ref); err != nil {
		return api.User{}, api.ApiKey{}, ErrInvalidCredentials
	}
	user, err := d.GetUserById(ref.UserID)
	if err != nil {
		return api.User{}, api.ApiKey{}, err
	}
	if isUserLocked(user) {
		return api.User{}, api.ApiKey{}, ErrUserLocked
	}
	if !user.Spec.Enabled {
		return api.User{}, api.ApiKey{}, ErrUserDisabled
	}
	var apiKey api.ApiKey
	resp, err := d.GetJSON(userApiKeyKey(ref.UserID, ref.ApiKeyID), &apiKey)
	if err != nil {
		return api.User{}, api.ApiKey{}, ErrInvalidCredentials
	}
	if apiKey.Spec.Revoked != nil && *apiKey.Spec.Revoked {
		return api.User{}, api.ApiKey{}, ErrInvalidCredentials
	}
	if apiKey.Status != nil && apiKey.Status.RevokedAt != nil {
		return api.User{}, api.ApiKey{}, ErrInvalidCredentials
	}
	if apiKey.Spec.ExpiresAt != nil && time.Now().After(*apiKey.Spec.ExpiresAt) {
		return api.User{}, api.ApiKey{}, ErrInvalidCredentials
	}
	if !isLoginSessionApiKey(apiKey) {
		if apiKey.Status == nil {
			apiKey.Status = &api.ApiKeyStatus{}
		}
		apiKey.Status.LastUsedAt = util.TimePtr(time.Now())
		if err := d.PutJSONCAS(userApiKeyKey(ref.UserID, ref.ApiKeyID), resp.Kvs[0].ModRevision, apiKey); err != nil {
			if err == ErrUpdateConflict {
				return api.User{}, api.ApiKey{}, ErrInvalidCredentials
			}
			slog.Warn("AuthenticateApiKey() failed to stamp last used", "err", err, "userId", ref.UserID, "apiKeyId", ref.ApiKeyID)
		}
		return user, apiKey, nil
	}
	now := time.Now()
	lastActivityAt := now
	if apiKey.Status != nil && apiKey.Status.LastUsedAt != nil {
		lastActivityAt = *apiKey.Status.LastUsedAt
	} else if apiKey.Spec.IssuedAt != nil {
		lastActivityAt = *apiKey.Spec.IssuedAt
	}
	if now.Sub(lastActivityAt) >= AuthSessionIdleTimeout {
		return api.User{}, api.ApiKey{}, ErrInvalidCredentials
	}
	if apiKey.Status == nil {
		apiKey.Status = &api.ApiKeyStatus{}
	}
	apiKey.Status.LastUsedAt = util.TimePtr(now)
	if err := d.PutJSONCAS(userApiKeyKey(ref.UserID, ref.ApiKeyID), resp.Kvs[0].ModRevision, apiKey); err != nil {
		if err == ErrUpdateConflict {
			return api.User{}, api.ApiKey{}, ErrInvalidCredentials
		}
		slog.Warn("AuthenticateApiKey() failed to stamp last used", "err", err, "userId", ref.UserID, "apiKeyId", ref.ApiKeyID)
	}
	return user, apiKey, nil
}
