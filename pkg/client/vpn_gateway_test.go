package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetVpnGatewayCertByIdSendsBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/vpn-gateway/1ec7a/cert" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"missing or invalid Authorization header"}`))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("client ovpn profile"))
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
		AccessToken: "session-token",
		Client:      server.Client(),
	}

	body, _, err := ep.GetVpnGatewayCertById("1ec7a")
	if err != nil {
		t.Fatalf("GetVpnGatewayCertById() failed: %v", err)
	}
	if string(body) != "client ovpn profile" {
		t.Fatalf("body = %q, want %q", string(body), "client ovpn profile")
	}
}

func TestGetVpnGatewayCertByIdFailsWithoutAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"missing or invalid Authorization header"}`))
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

	if _, _, err := ep.GetVpnGatewayCertById("1ec7a"); err == nil {
		t.Fatalf("GetVpnGatewayCertById() expected error without access token")
	}
}
