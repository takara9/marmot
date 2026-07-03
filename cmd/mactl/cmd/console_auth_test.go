package cmd

import (
	"net/http"
	"testing"
)

func TestConsoleRequestAddsAuthorizationHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/v1/server/abc/console", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Host = "example.com"
	req.Header.Set("Connection", "close")
	setConsoleAuthorizationHeader(req, "token-123")

	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer token-123")
	}
}

func TestConsoleRequestKeepsExistingAuthorizationHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/v1/server/abc/console", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer existing")

	setConsoleAuthorizationHeader(req, "token-123")

	if got := req.Header.Get("Authorization"); got != "Bearer existing" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer existing")
	}
}

func TestConsoleRequestSkipsEmptyAuthorizationToken(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/v1/server/abc/console", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}

	setConsoleAuthorizationHeader(req, "   ")

	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization header = %q, want empty", got)
	}
}

func TestConsoleRequestTrimsAuthorizationToken(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/api/v1/server/abc/console", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}

	setConsoleAuthorizationHeader(req, "  token-123  ")

	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer token-123")
	}
}
