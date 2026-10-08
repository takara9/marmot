package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetServerConsoleGraphicalAtUsesGivenHostPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/e934d/console/graphical" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host":"192.168.1.70","port":5905,"passwd":"s3cret"}`))
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	ep := &MarmotEndpoint{
		// Deliberately set HostPort to an unused address; GetServerConsoleGraphicalAt
		// must use the hostPort argument instead (multi-node routing).
		Scheme:   parsedURL.Scheme,
		HostPort: "127.0.0.1:1",
		BasePath: "/api/v1",
		Client:   server.Client(),
	}

	body, _, err := ep.GetServerConsoleGraphicalAt(parsedURL.Host, "e934d")
	if err != nil {
		t.Fatalf("GetServerConsoleGraphicalAt() failed: %v", err)
	}
	want := `{"host":"192.168.1.70","port":5905,"passwd":"s3cret"}`
	if string(body) != want {
		t.Fatalf("body = %q, want %q", string(body), want)
	}
}

func TestGetServerConsoleGraphicalAtFallsBackToEndpointHostPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"host":"127.0.0.1","port":5900}`))
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

	if _, _, err := ep.GetServerConsoleGraphicalAt("", "e934d"); err != nil {
		t.Fatalf("GetServerConsoleGraphicalAt() failed: %v", err)
	}
}
