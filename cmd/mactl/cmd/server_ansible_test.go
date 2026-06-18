package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestValidateServerAnsibleSpec(t *testing.T) {
	playbook := "playbook/setup.yaml"
	inventory := "hosts"
	ip := "192.168.1.64"

	tests := []struct {
		name    string
		server  api.Server
		wantIP  string
		wantErr string
	}{
		{
			name:   "ansible not specified",
			server: api.Server{Spec: api.ServerSpec{}},
			wantIP: "",
		},
		{
			name: "ansible with host-bridge and address",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook:  playbook,
					Inventory: inventory,
				},
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
				Ansible: &api.ServerAnsible{
					Playbook:  playbook,
					Inventory: inventory,
				},
			}},
			wantErr: "requires spec.networkInterface",
		},
		{
			name: "missing playbook",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Inventory: inventory,
				},
			}},
			wantErr: "spec.ansible.playbook is required",
		},
		{
			name: "missing inventory",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook: playbook,
				},
			}},
			wantErr: "spec.ansible.inventory is required",
		},
		{
			name: "host-bridge without address",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook:  playbook,
					Inventory: inventory,
				},
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
				}},
			}},
			wantErr: "requires host-bridge address",
		},
		{
			name: "host-bridge missing",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook:  playbook,
					Inventory: inventory,
				},
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
			gotIP, err := validateServerAnsibleSpec(tt.server)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateServerAnsibleSpec() unexpected err: %v", err)
				}
				if gotIP != tt.wantIP {
					t.Fatalf("validateServerAnsibleSpec() ip = %q, want %q", gotIP, tt.wantIP)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateServerAnsibleSpec() expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateServerAnsibleSpec() err = %q, want contains %q", err.Error(), tt.wantErr)
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

	id, err = extractSuccessID([]byte(`{"id":null,"metadata":{"id":"s5678"},"message":"ok"}`))
	if err != nil {
		t.Fatalf("extractSuccessID() unexpected err: %v", err)
	}
	if id != "s5678" {
		t.Fatalf("extractSuccessID() = %q, want s5678", id)
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

func TestResolveServerAnsibleInventoryPathRelative(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	relPath := "hosts"
	if err := os.WriteFile(relPath, []byte("[all]\nserver ansible_host=192.168.1.64\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	path, err := resolveServerAnsibleInventoryPath(relPath)
	if err != nil {
		t.Fatalf("resolveServerAnsibleInventoryPath() unexpected err: %v", err)
	}
	if path != filepath.Join(tmp, relPath) {
		t.Fatalf("resolveServerAnsibleInventoryPath() = %q, want %q", path, filepath.Join(tmp, relPath))
	}
}

func TestValidateServerAnsibleSpecUsesStrings(t *testing.T) {
	playbook := "playbook/setup.yaml"
	inventory := "hosts"
	ip := "192.168.1.64"
	server := api.Server{Spec: api.ServerSpec{
		Ansible: &api.ServerAnsible{
			Playbook:  playbook,
			Inventory: inventory,
		},
		NetworkInterface: &[]api.NetworkInterface{{
			Networkname: "host-bridge",
			Address:     &ip,
		}},
	}}
	got, err := validateServerAnsibleSpec(server)
	if err != nil {
		t.Fatalf("validateServerAnsibleSpec() unexpected err: %v", err)
	}
	if got != ip {
		t.Fatalf("validateServerAnsibleSpec() = %q, want %q", got, ip)
	}
}

func TestRunServerAnsiblePlaybookWithExtraArgs(t *testing.T) {
	original := serverAnsibleExecCommand
	t.Cleanup(func() {
		serverAnsibleExecCommand = original
	})

	var gotName string
	var gotArgs []string
	serverAnsibleExecCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string{}, args...)
		return exec.Command("true")
	}

	extraArgs := []string{"flush-cache", `tags "nginx,mysql"`, "", "   ", "--skip-tags=cache"}
	err := runServerAnsiblePlaybook("/tmp/playbook.yaml", "/tmp/hosts", "/tmp/id_test", &extraArgs)
	if err != nil {
		t.Fatalf("runServerAnsiblePlaybook() unexpected err: %v", err)
	}

	if gotName != "ansible-playbook" {
		t.Fatalf("command name = %q, want ansible-playbook", gotName)
	}
	wantArgs := []string{
		"-i", "/tmp/hosts",
		"/tmp/playbook.yaml",
		"--private-key", "/tmp/id_test",
		"--flush-cache",
		"--tags",
		"nginx,mysql",
		"--skip-tags=cache",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunServerAnsiblePlaybookWithInvalidExtraArgs(t *testing.T) {
	extraArgs := []string{`tags "nginx,mysql`}
	err := runServerAnsiblePlaybook("/tmp/playbook.yaml", "/tmp/hosts", "/tmp/id_test", &extraArgs)
	if err == nil {
		t.Fatalf("runServerAnsiblePlaybook() expected error for invalid extra args")
	}
	if !strings.Contains(err.Error(), "invalid spec.ansible.extra-args entry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunServerAnsiblePingWithoutUserOption(t *testing.T) {
	original := serverAnsibleExecCommand
	t.Cleanup(func() {
		serverAnsibleExecCommand = original
	})

	var gotName string
	var gotArgs []string
	serverAnsibleExecCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string{}, args...)
		return exec.Command("true")
	}

	err := runServerAnsiblePing("192.168.1.64", "/tmp/id_test")
	if err != nil {
		t.Fatalf("runServerAnsiblePing() unexpected err: %v", err)
	}

	if gotName != "ansible" {
		t.Fatalf("command name = %q, want ansible", gotName)
	}
	wantArgs := []string{
		"all",
		"-i", "192.168.1.64,",
		"-m", "ping",
		"--private-key", "/tmp/id_test",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
}
