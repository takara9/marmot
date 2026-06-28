package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestAuthLoginStoresTokenAndUsesBearerAuth(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for login: %s", r.Method)
			}
			var request api.AuthLoginRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode login request: %v", err)
			}
			if request.UserId != "admin" || request.Password != "passw0rd" {
				t.Fatalf("unexpected login request: %+v", request)
			}
			response := api.AuthLoginResponse{
				AccessToken: "session-token",
				TokenType:   "Bearer",
				User: &api.User{
					ApiVersion: "v1",
					Kind:       "User",
					Metadata:   api.Metadata{Id: "admin", Name: "admin"},
					Spec:       api.UserSpec{Enabled: true},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		case "/api/v1/auth/me":
			if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			response := api.AuthMe{UserId: "admin", Roles: []string{"Administrator"}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	ep := &MarmotEndpoint{
		Scheme:   parsedURL.Scheme,
		HostPort: parsedURL.Host,
		BasePath: "/api/v1",
		Client:   server.Client(),
	}

	loginResp, err := ep.AuthLogin("admin", "passw0rd")
	if err != nil {
		t.Fatalf("AuthLogin failed: %v", err)
	}
	if loginResp == nil || loginResp.AccessToken != "session-token" {
		t.Fatalf("unexpected login response: %+v", loginResp)
	}

	meResp, err := ep.AuthMe()
	if err != nil {
		t.Fatalf("AuthMe failed: %v", err)
	}
	if meResp == nil || meResp.UserId != "admin" {
		t.Fatalf("unexpected me response: %+v", meResp)
	}
}

func TestCreateUserApiKeyReturnsRawToken(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/alice/apikeys" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		var request api.ApiKeyCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode api key request: %v", err)
		}
		if request.Comment == nil || strings.TrimSpace(*request.Comment) != "login-session" {
			t.Fatalf("unexpected request comment: %+v", request.Comment)
		}
		response := api.ApiKey{
			ApiVersion: "v1",
			Kind:       "ApiKey",
			Metadata:   api.Metadata{Id: "key-1", Name: "key-1"},
			Spec:       api.ApiKeySpec{Comment: request.Comment},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Marmot-Api-Key", "raw-token-value")
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	ep := &MarmotEndpoint{
		Scheme:      parsedURL.Scheme,
		HostPort:    parsedURL.Host,
		BasePath:    "/api/v1",
		AccessToken: "admin-token",
		Client:      server.Client(),
	}

	apiKey, rawToken, err := ep.CreateUserApiKey("alice", api.ApiKeyCreateRequest{Comment: stringPtr("login-session")})
	if err != nil {
		t.Fatalf("CreateUserApiKey failed: %v", err)
	}
	if apiKey == nil || apiKey.Metadata.Id != "key-1" {
		t.Fatalf("unexpected api key response: %+v", apiKey)
	}
	if rawToken != "raw-token-value" {
		t.Fatalf("unexpected raw token: %q", rawToken)
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestSetAccessTokenTrimsWhitespace(t *testing.T) {
	t.Helper()
	ep := &MarmotEndpoint{}
	ep.SetAccessToken("  token-value  ")
	if ep.AccessToken != "token-value" {
		t.Fatalf("unexpected access token: %q", ep.AccessToken)
	}
}
