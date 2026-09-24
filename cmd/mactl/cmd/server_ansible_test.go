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

	tests := []struct {
		name    string
		server  api.Server
		wantErr string
	}{
		{
			name:   "ansible not specified",
			server: api.Server{Spec: api.ServerSpec{}},
		},
		{
			name: "ansible with host-bridge without address is accepted",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook:  strRef(playbook),
					Inventory: strRef(inventory),
				},
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
				}},
			}},
		},
		{
			name: "missing network interface",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook:  strRef(playbook),
					Inventory: strRef(inventory),
				},
			}},
			wantErr: "requires spec.networkInterface with host-bridge",
		},
		{
			name: "missing playbook",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Inventory: strRef(inventory),
				},
			}},
			wantErr: "spec.ansible.playbook is required",
		},
		{
			name: "missing inventory",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook: strRef(playbook),
				},
			}},
			wantErr: "spec.ansible.inventory is required",
		},
		{
			name: "host-bridge missing",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Playbook:  strRef(playbook),
					Inventory: strRef(inventory),
				},
				NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "default",
				}},
			}},
			wantErr: "only when host-bridge is specified",
		},
		{
			name: "onBoot with remotePlaybook is accepted",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					OnBoot:         boolPtr(true),
					RemotePlaybook: strRef("https://example.com/playbook.yaml"),
				},
			}},
		},
		{
			name: "onBoot with ansible pull is accepted",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					OnBoot: boolPtr(true),
					Pull: &api.ServerAnsiblePull{
						Url:          strRef("https://github.com/example/repo.git"),
						PlaybookYaml: strRef("site.yml"),
					},
				},
			}},
		},
		{
			name: "onBoot requires remotePlaybook or pull url",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					OnBoot: boolPtr(true),
				},
			}},
			wantErr: "remotePlaybook or spec.ansible.pull.url is required",
		},
		{
			name: "remotePlaybook requires onBoot",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					RemotePlaybook: strRef("https://example.com/playbook.yaml"),
				},
			}},
			wantErr: "requires spec.ansible.onBoot=true",
		},
		{
			name: "pull url requires onBoot",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					Pull: &api.ServerAnsiblePull{
						Url: strRef("https://github.com/example/repo.git"),
					},
				},
			}},
			wantErr: "spec.ansible.pull.url requires spec.ansible.onBoot=true",
		},
		{
			name: "onBoot cannot combine remotePlaybook and pull url",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					OnBoot:         boolPtr(true),
					RemotePlaybook: strRef("https://example.com/playbook.yaml"),
					Pull: &api.ServerAnsiblePull{
						Url: strRef("https://github.com/example/repo.git"),
					},
				},
			}},
			wantErr: "cannot be combined",
		},
		{
			name: "onBoot cannot be combined with local playbook/inventory",
			server: api.Server{Spec: api.ServerSpec{
				Ansible: &api.ServerAnsible{
					OnBoot:         boolPtr(true),
					RemotePlaybook: strRef("https://example.com/playbook.yaml"),
					Playbook:       strRef(playbook),
					Inventory:      strRef(inventory),
				},
			}},
			wantErr: "cannot be combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServerAnsibleSpec(tt.server)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateServerAnsibleSpec() unexpected err: %v", err)
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

func boolPtr(v bool) *bool {
	return &v
}

func strRef(v string) *string {
	return &v
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

	got, err := resolveServerAnsiblePrivateKeyPaths()
	if err != nil {
		t.Fatalf("resolveServerAnsiblePrivateKeyPaths() unexpected err: %v", err)
	}
	if len(got) != 1 || got[0] != keyPath {
		t.Fatalf("resolveServerAnsiblePrivateKeyPaths() = %v, want [%q]", got, keyPath)
	}
}

// TestResolveServerAnsiblePrivateKeyPathsReturnsAllCandidates は、~/.ssh に
// id_rsa と id_ed25519 が両方存在する場合に、両方が候補として返ることを検証する(issue #723)。
func TestResolveServerAnsiblePrivateKeyPathsReturnsAllCandidates(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	ed25519Path := filepath.Join(sshDir, "id_ed25519")
	rsaPath := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(ed25519Path, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := os.WriteFile(rsaPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	old := os.Getenv(serverAnsiblePrivateKeyEnvName)
	_ = os.Unsetenv(serverAnsiblePrivateKeyEnvName)
	t.Cleanup(func() {
		_ = os.Setenv(serverAnsiblePrivateKeyEnvName, old)
	})
	t.Setenv("HOME", tmp)

	got, err := resolveServerAnsiblePrivateKeyPaths()
	if err != nil {
		t.Fatalf("resolveServerAnsiblePrivateKeyPaths() unexpected err: %v", err)
	}
	want := []string{ed25519Path, rsaPath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveServerAnsiblePrivateKeyPaths() = %v, want %v", got, want)
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

	env := serverAnsibleCommandEnv([]string{"/tmp/id_test"})
	if !containsPrefix(env, "ANSIBLE_HOST_KEY_CHECKING=False") {
		t.Fatalf("serverAnsibleCommandEnv() should include ANSIBLE_HOST_KEY_CHECKING when ansible.cfg is absent")
	}
	if !containsPrefix(env, "ANSIBLE_REMOTE_TEMP=/tmp") {
		t.Fatalf("serverAnsibleCommandEnv() should include ANSIBLE_REMOTE_TEMP=/tmp when ansible.cfg is absent")
	}
}

// TestServerAnsibleCommandEnvWithMultipleKeys は、鍵候補が複数ある場合に
// 全候補が ANSIBLE_SSH_ARGS の IdentityFile として渡されることを検証する(issue #723)。
func TestServerAnsibleCommandEnvWithMultipleKeys(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	env := serverAnsibleCommandEnv([]string{"/tmp/id_ed25519", "/tmp/id_rsa"})
	sshArgs := ""
	for _, item := range env {
		if strings.HasPrefix(item, "ANSIBLE_SSH_ARGS=") {
			sshArgs = strings.TrimPrefix(item, "ANSIBLE_SSH_ARGS=")
			break
		}
	}

	tokens, err := splitServerAnsibleExtraArg(sshArgs)
	if err != nil {
		t.Fatalf("splitServerAnsibleExtraArg() unexpected err: %v", err)
	}
	if !reflect.DeepEqual(tokens, []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=60s",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "IdentitiesOnly=yes",
		"-i", "/tmp/id_ed25519",
		"-i", "/tmp/id_rsa",
	}) {
		t.Fatalf("splitServerAnsibleExtraArg(%q) = %#v", sshArgs, tokens)
	}
	if !strings.Contains(sshArgs, "IdentitiesOnly=yes") {
		t.Fatalf("ANSIBLE_SSH_ARGS = %q, want IdentitiesOnly=yes", sshArgs)
	}
}

func TestServerAnsibleCommandEnvWithMultipleKeysContainingWhitespace(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	keyWithWhitespace := "/tmp/Test User/id_ed25519"
	env := serverAnsibleCommandEnv([]string{keyWithWhitespace, "/tmp/id_rsa"})
	sshArgs := ""
	for _, item := range env {
		if strings.HasPrefix(item, "ANSIBLE_SSH_ARGS=") {
			sshArgs = strings.TrimPrefix(item, "ANSIBLE_SSH_ARGS=")
			break
		}
	}

	tokens, err := splitServerAnsibleExtraArg(sshArgs)
	if err != nil {
		t.Fatalf("splitServerAnsibleExtraArg() unexpected err: %v", err)
	}
	if !reflect.DeepEqual(tokens, []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=60s",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "IdentitiesOnly=yes",
		"-i", keyWithWhitespace,
		"-i", "/tmp/id_rsa",
	}) {
		t.Fatalf("splitServerAnsibleExtraArg(%q) = %#v", sshArgs, tokens)
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
	env := serverAnsibleCommandEnv([]string{"/tmp/id_test"})
	if containsPrefix(env, "ANSIBLE_HOST_KEY_CHECKING=False") {
		t.Fatalf("serverAnsibleCommandEnv() must not inject ansible env vars when ansible.cfg exists")
	}
	if containsPrefix(env, "ANSIBLE_REMOTE_TEMP=/tmp") {
		t.Fatalf("serverAnsibleCommandEnv() must not inject ANSIBLE_REMOTE_TEMP when ansible.cfg exists")
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
	server := api.Server{Spec: api.ServerSpec{
		Ansible: &api.ServerAnsible{
			Playbook:  strRef(playbook),
			Inventory: strRef(inventory),
		},
		NetworkInterface: &[]api.NetworkInterface{{
			Networkname: "host-bridge",
		}},
	}}
	err := validateServerAnsibleSpec(server)
	if err != nil {
		t.Fatalf("validateServerAnsibleSpec() unexpected err: %v", err)
	}
}

func TestHostBridgeAddressFromServer(t *testing.T) {
	ip := "192.168.1.64"

	t.Run("address is resolved", func(t *testing.T) {
		nics := []api.NetworkInterface{{Networkname: "host-bridge", Address: &ip}}
		server := api.Server{Spec: api.ServerSpec{NetworkInterface: &nics}}

		got, err := hostBridgeAddressFromServer(server)
		if err != nil {
			t.Fatalf("hostBridgeAddressFromServer() unexpected err: %v", err)
		}
		if got != ip {
			t.Fatalf("hostBridgeAddressFromServer() = %q, want %q", got, ip)
		}
	})

	t.Run("host-bridge without address returns error", func(t *testing.T) {
		nics := []api.NetworkInterface{{Networkname: "host-bridge"}}
		server := api.Server{Spec: api.ServerSpec{NetworkInterface: &nics}}

		_, err := hostBridgeAddressFromServer(server)
		if err == nil {
			t.Fatalf("hostBridgeAddressFromServer() expected error")
		}
		if !strings.Contains(err.Error(), "not assigned yet") {
			t.Fatalf("hostBridgeAddressFromServer() err = %q, want contains %q", err.Error(), "not assigned yet")
		}
	})
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
	err := runServerAnsiblePlaybook("/tmp/playbook.yaml", "/tmp/hosts", []string{"/tmp/id_test"}, &extraArgs)
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

// TestRunServerAnsiblePlaybookWithMultipleKeys は、鍵候補が複数ある場合に
// --private-key を使わず全候補を SSH に委ねることを検証する(issue #723)。
func TestRunServerAnsiblePlaybookWithMultipleKeys(t *testing.T) {
	original := serverAnsibleExecCommand
	t.Cleanup(func() {
		serverAnsibleExecCommand = original
	})

	var gotArgs []string
	serverAnsibleExecCommand = func(name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		return exec.Command("true")
	}

	err := runServerAnsiblePlaybook("/tmp/playbook.yaml", "/tmp/hosts", []string{"/tmp/id_ed25519", "/tmp/id_rsa"}, nil)
	if err != nil {
		t.Fatalf("runServerAnsiblePlaybook() unexpected err: %v", err)
	}

	wantArgs := []string{"-i", "/tmp/hosts", "/tmp/playbook.yaml"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v (no --private-key when multiple key candidates exist)", gotArgs, wantArgs)
	}
}

func TestRunServerAnsiblePlaybookWithInvalidExtraArgs(t *testing.T) {
	extraArgs := []string{`tags "nginx,mysql`}
	err := runServerAnsiblePlaybook("/tmp/playbook.yaml", "/tmp/hosts", []string{"/tmp/id_test"}, &extraArgs)
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

	err := runServerAnsiblePing("192.168.1.64", []string{"/tmp/id_test"})
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
