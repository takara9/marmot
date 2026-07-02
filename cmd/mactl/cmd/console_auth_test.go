package cmd

import (
	"net/http"
	"testing"

	"github.com/takara9/marmot/pkg/client"
)

func TestConsoleRequestAddsAuthorizationHeader(t *testing.T) {
	m := &client.MarmotEndpoint{AccessToken: "token-123"}
	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/v1/server/abc/console", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Host = "example.com"
	req.Header.Set("Connection", "close")
	if token := m.AccessToken; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer token-123")
	}
}
