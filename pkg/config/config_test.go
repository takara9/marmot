package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReadYamlConfigFromFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "server.yaml")
	content := "name: file-server\ncpu: 2\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	var conf Server
	if err := ReadYamlConfig(configPath, &conf); err != nil {
		t.Fatalf("ReadYamlConfig returned error: %v", err)
	}

	if conf.Name != "file-server" {
		t.Fatalf("unexpected server name: %q", conf.Name)
	}
	if conf.Cpu == nil || *conf.Cpu != 2 {
		t.Fatalf("unexpected cpu value: %#v", conf.Cpu)
	}
}

func TestReadYamlConfigFromURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server.yaml" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("name: url-server\nmemory: 2048\n"))
	}))
	defer server.Close()

	var conf Server
	if err := ReadYamlConfig(server.URL+"/server.yaml", &conf); err != nil {
		t.Fatalf("ReadYamlConfig returned error: %v", err)
	}

	if conf.Name != "url-server" {
		t.Fatalf("unexpected server name: %q", conf.Name)
	}
	if conf.Memory == nil || *conf.Memory != 2048 {
		t.Fatalf("unexpected memory value: %#v", conf.Memory)
	}
}

func TestReadYamlConfigFromURLStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	var conf Server
	err := ReadYamlConfig(server.URL+"/missing.yaml", &conf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadYamlConfigFromURLInvalidYAML(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("name: [unterminated\n"))
	}))
	defer server.Close()

	var conf Server
	err := ReadYamlConfig(server.URL, &conf)
	if err == nil {
		t.Fatal("expected yaml unmarshal error, got nil")
	}
}
