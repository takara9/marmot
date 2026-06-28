package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/takara9/marmot/api"
)

func (m *MarmotEndpoint) SetAccessToken(token string) {
	m.AccessToken = strings.TrimSpace(token)
}

func (m *MarmotEndpoint) endpointURL(pathSegments ...string) (string, error) {
	parts := []string{m.Scheme + "://" + m.HostPort, m.BasePath}
	parts = append(parts, pathSegments...)
	return url.JoinPath(parts[0], parts[1:]...)
}

func (m *MarmotEndpoint) newJSONRequest(method string, body any, pathSegments ...string) (*http.Request, error) {
	reqURL, err := m.endpointURL(pathSegments...)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	return http.NewRequest(method, reqURL, reader)
}

func (m *MarmotEndpoint) doJSONRequest(req *http.Request) (int, []byte, http.Header, *url.URL, error) {
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	if strings.TrimSpace(req.Header.Get("Content-Type")) == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := strings.TrimSpace(m.AccessToken); token != "" && strings.TrimSpace(req.Header.Get("Authorization")) == "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := m.Client.Do(req)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, nil, err
	}

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusAccepted &&
		resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(body))
		var apiErr apiErrorBody
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if strings.TrimSpace(apiErr.Message) != "" {
				return resp.StatusCode, nil, resp.Header, nil, fmt.Errorf("%s", strings.TrimSpace(apiErr.Message))
			}
		}
		if len(msg) > 0 {
			return resp.StatusCode, nil, resp.Header, nil, fmt.Errorf("%s", msg)
		}
		return resp.StatusCode, nil, resp.Header, nil, fmt.Errorf("http status code = %d", resp.StatusCode)
	}

	location, err := resp.Location()
	if err != nil && err.Error() != "http: no Location header in response" {
		return resp.StatusCode, body, resp.Header, nil, err
	}
	if err != nil {
		location = nil
	}
	return resp.StatusCode, body, resp.Header, location, nil
}

func decodeJSONBody[T any](body []byte) (*T, error) {
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (m *MarmotEndpoint) AuthLogin(userID, password string) (*api.AuthLoginResponse, error) {
	requestBody := api.AuthLoginRequest{UserId: strings.TrimSpace(userID), Password: password}
	req, err := m.newJSONRequest(http.MethodPost, requestBody, "auth", "login")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := decodeJSONBody[api.AuthLoginResponse](body)
	if err != nil {
		return nil, err
	}
	m.SetAccessToken(resp.AccessToken)
	return resp, nil
}

func (m *MarmotEndpoint) AuthLogout() error {
	req, err := m.newJSONRequest(http.MethodPost, nil, "auth", "logout")
	if err != nil {
		return err
	}
	if _, _, _, _, err := m.doJSONRequest(req); err != nil {
		return err
	}
	m.SetAccessToken("")
	return nil
}

func (m *MarmotEndpoint) AuthMe() (*api.AuthMe, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "auth", "me")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[api.AuthMe](body)
}

func (m *MarmotEndpoint) AuthzCheck(request api.AuthzCheckRequest) (*api.AuthzCheckResponse, error) {
	req, err := m.newJSONRequest(http.MethodPost, request, "authz", "check")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[api.AuthzCheckResponse](body)
}

func (m *MarmotEndpoint) ListRoles() (api.Roles, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "roles")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	var roles api.Roles
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (m *MarmotEndpoint) GetRoleByName(roleName string) (*api.Role, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "roles", roleName)
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[api.Role](body)
}

func (m *MarmotEndpoint) ListUsers() (api.Users, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "users")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	var users api.Users
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (m *MarmotEndpoint) CreateUser(user api.User) (*api.User, error) {
	req, err := m.newJSONRequest(http.MethodPost, user, "users")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[api.User](body)
}

func (m *MarmotEndpoint) GetUserById(userID string) (*api.User, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "users", userID)
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[api.User](body)
}

func (m *MarmotEndpoint) UpdateUserById(userID string, user api.User) (*api.User, error) {
	req, err := m.newJSONRequest(http.MethodPut, user, "users", userID)
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[api.User](body)
}

func (m *MarmotEndpoint) DeleteUserById(userID string) error {
	req, err := m.newJSONRequest(http.MethodDelete, nil, "users", userID)
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}

func (m *MarmotEndpoint) ChangeUserPassword(userID string, request api.PasswordChangeRequest) error {
	req, err := m.newJSONRequest(http.MethodPost, request, "users", userID, "password")
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}

func (m *MarmotEndpoint) LockUserById(userID string) error {
	req, err := m.newJSONRequest(http.MethodPost, nil, "users", userID, "lock")
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}

func (m *MarmotEndpoint) UnlockUserById(userID string) error {
	req, err := m.newJSONRequest(http.MethodPost, nil, "users", userID, "unlock")
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}

func (m *MarmotEndpoint) ListUserRoles(userID string) (api.RoleNames, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "users", userID, "roles")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	var roles api.RoleNames
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (m *MarmotEndpoint) AddUserRole(userID string, roleName string) error {
	req, err := m.newJSONRequest(http.MethodPost, api.RoleAssignmentRequest{RoleName: roleName}, "users", userID, "roles")
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}

func (m *MarmotEndpoint) DeleteUserRole(userID string, roleName string) error {
	req, err := m.newJSONRequest(http.MethodDelete, nil, "users", userID, "roles", roleName)
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}

func (m *MarmotEndpoint) ListUserApiKeys(userID string) (api.ApiKeys, error) {
	req, err := m.newJSONRequest(http.MethodGet, nil, "users", userID, "apikeys")
	if err != nil {
		return nil, err
	}
	_, body, _, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, err
	}
	var keys api.ApiKeys
	if err := json.Unmarshal(body, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func (m *MarmotEndpoint) CreateUserApiKey(userID string, request api.ApiKeyCreateRequest) (*api.ApiKey, string, error) {
	req, err := m.newJSONRequest(http.MethodPost, request, "users", userID, "apikeys")
	if err != nil {
		return nil, "", err
	}
	_, body, header, _, err := m.doJSONRequest(req)
	if err != nil {
		return nil, "", err
	}
	apiKey, err := decodeJSONBody[api.ApiKey](body)
	if err != nil {
		return nil, "", err
	}
	rawToken := strings.TrimSpace(header.Get("X-Marmot-Api-Key"))
	if rawToken == "" {
		return nil, "", fmt.Errorf("missing X-Marmot-Api-Key header")
	}
	return apiKey, rawToken, nil
}

func (m *MarmotEndpoint) DeleteUserApiKey(userID string, apiKeyID string) error {
	req, err := m.newJSONRequest(http.MethodDelete, nil, "users", userID, "apikeys", apiKeyID)
	if err != nil {
		return err
	}
	_, _, _, _, err = m.doJSONRequest(req)
	return err
}
