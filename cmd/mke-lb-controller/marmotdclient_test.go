package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMarmotdClientAcceptsCustomCA(t *testing.T) {
	apiKey := "test-apikey"
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer ts.Close()

	caFile := filepath.Join(t.TempDir(), "marmotd-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("failed to write CA file: %v", err)
	}
	apiKeyFile := filepath.Join(t.TempDir(), "marmotd-apikey")
	if err := os.WriteFile(apiKeyFile, []byte(apiKey+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write API key file: %v", err)
	}

	client, err := loadMarmotdClient(ts.URL, apiKeyFile, "ke-demo", caFile)
	if err != nil {
		t.Fatalf("loadMarmotdClient() failed: %v", err)
	}
	if _, err := client.hostBridgeAddressesByServerName(context.Background()); err != nil {
		t.Fatalf("hostBridgeAddressesByServerName() failed with custom CA: %v", err)
	}
}
