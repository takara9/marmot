package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestValidateServerAnsiblePlaybookSpec(t *testing.T) {
	playbook := "playbook/setup.yaml"
	ip := "192.168.1.64"

	tests := []struct {
		name    string
		server  api.Server
		wantIP  string
		wantErr string
	}{
		{
			name:   "playbook not specified",
			server: api.Server{Spec: api.ServerSpec{}},
			wantIP: "",
		},
		{
			name: "host-bridge with address",
			server: api.Server{Spec: api.ServerSpec{
				AnsiblePlaybook: &playbook,
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
					Address:     &ip,
				}},
			}},
			wantIP: ip,
		},
		{
			name: "missing network interface",
			server: api.Server{Spec: api.ServerSpec{
				AnsiblePlaybook: &playbook,
			}},
			wantErr: "requires spec.networkInterface",
		},
		{
			name: "host-bridge without address",
			server: api.Server{Spec: api.ServerSpec{
				AnsiblePlaybook: &playbook,
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
				}},
			}},
			wantErr: "requires host-bridge address",
		},
		{
			name: "host-bridge missing",
			server: api.Server{Spec: api.ServerSpec{
				AnsiblePlaybook: &playbook,
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "default",
					Address:     &ip,
				}},
			}},
			wantErr: "only when host-bridge is specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, err := validateServerAnsiblePlaybookSpec(tt.server)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateServerAnsiblePlaybookSpec() unexpected err: %v", err)
				}
				if gotIP != tt.wantIP {
					t.Fatalf("validateServerAnsiblePlaybookSpec() ip = %q, want %q", gotIP, tt.wantIP)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateServerAnsiblePlaybookSpec() expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateServerAnsiblePlaybookSpec() err = %q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestExtractSuccessID(t *testing.T) {
	id, err := extractSuccessID([]byte(`{"id":"s1234","message":"ok"}`))
	if err != nil {
		t.Fatalf("extractSuccessID() unexpected err: %v", err)
	}
	if id != "s1234" {
		t.Fatalf("extractSuccessID() = %q, want s1234", id)
	}

	if _, err := extractSuccessID([]byte(`{"message":"ok"}`)); err == nil {
		t.Fatalf("extractSuccessID() expected error for missing id")
	}
}

func TestResolveServerAnsiblePrivateKeyPathWithEnv(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_test")
	if err := os.WriteFile(keyPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	old := os.Getenv(serverAnsiblePrivateKeyEnvName)
	t.Cleanup(func() {
		_ = os.Setenv(serverAnsiblePrivateKeyEnvName, old)
	})
	if err := os.Setenv(serverAnsiblePrivateKeyEnvName, keyPath); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}

	got, err := resolveServerAnsiblePrivateKeyPath()
	if err != nil {
		t.Fatalf("resolveServerAnsiblePrivateKeyPath() unexpected err: %v", err)
	}
	if got != keyPath {
		t.Fatalf("resolveServerAnsiblePrivateKeyPath() = %q, want %q", got, keyPath)
	}
}

func TestServerAnsibleCommandEnvWithoutConfig(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	env := serverAnsibleCommandEnv()
	if !containsPrefix(env, "ANSIBLE_HOST_KEY_CHECKING=False") {
		t.Fatalf("serverAnsibleCommandEnv() should include ANSIBLE_HOST_KEY_CHECKING when ansible.cfg is absent")
	}
}

func TestServerAnsibleCommandEnvWithConfig(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if err := os.WriteFile("ansible.cfg", []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	env := serverAnsibleCommandEnv()
	if containsPrefix(env, "ANSIBLE_HOST_KEY_CHECKING=False") {
		t.Fatalf("serverAnsibleCommandEnv() must not inject ansible env vars when ansible.cfg exists")
	}
}

func containsPrefix(items []string, prefix string) bool {
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func TestResolveServerAnsiblePlaybookPathRelative(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	relPath := filepath.Join("playbook", "setup.yaml")
	if err := os.MkdirAll(filepath.Dir(relPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	if err := os.WriteFile(relPath, []byte("---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	path, err := resolveServerAnsiblePlaybookPath(relPath)
	if err != nil {
		t.Fatalf("resolveServerAnsiblePlaybookPath() unexpected err: %v", err)
	}
	if path != filepath.Join(tmp, relPath) {
		t.Fatalf("resolveServerAnsiblePlaybookPath() = %q, want %q", path, filepath.Join(tmp, relPath))
	}
}

func TestValidateServerAnsiblePlaybookSpecUsesStringPtr(t *testing.T) {
	playbook := util.StringPtr("playbook/setup.yaml")
	ip := util.StringPtr("192.168.1.64")
	server := api.Server{Spec: api.ServerSpec{
		AnsiblePlaybook: playbook,
		NetworkInterface: &[]api.NetworkInterface{{
			Networkname: "host-bridge",
			Address:     ip,
		}},
	}}
	got, err := validateServerAnsiblePlaybookSpec(server)
	if err != nil {
		t.Fatalf("validateServerAnsiblePlaybookSpec() unexpected err: %v", err)
	}
	if got != *ip {
		t.Fatalf("validateServerAnsiblePlaybookSpec() = %q, want %q", got, *ip)
	}
}
