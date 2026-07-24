package marmotd

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestBuildCloudInitUserDataMultipleUsers(t *testing.T) {
	got, err := buildCloudInitUserData("12345", "ssh-rsa AAAA\nssh-ed25519 BBBB", []string{"root", "ubuntu"}, nil)
	if err != nil {
		t.Fatalf("buildCloudInitUserData() unexpected err: %v", err)
	}
	want := `#cloud-config
users:
  - name: root
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAA
      - ssh-ed25519 BBBB
  - name: ubuntu
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAA
      - ssh-ed25519 BBBB
chpasswd:
  list:
    - root:12345
    - ubuntu:12345
  expire: False
`
	if got != want {
		t.Fatalf("buildCloudInitUserData() = %q, want %q", got, want)
	}
}

func TestBuildCloudInitUserDataAnsibleOnBoot(t *testing.T) {
	onBoot := true
	url := "https://example.com/playbook.yaml"
	data, err := buildCloudInitUserData("", "ssh-rsa AAAA", []string{"ubuntu"}, &api.ServerAnsible{
		OnBoot:         &onBoot,
		RemotePlaybook: &url,
	})
	if err != nil {
		t.Fatalf("buildCloudInitUserData() unexpected err: %v", err)
	}
	if !strings.Contains(data, "runcmd:") {
		t.Fatalf("user-data must contain runcmd for onBoot ansible")
	}
	if !strings.Contains(data, "ansible-playbook -i localhost, -c local") {
		t.Fatalf("user-data must execute ansible-playbook in local mode")
	}
	if !strings.Contains(data, "https://example.com/playbook.yaml") {
		t.Fatalf("user-data must contain remote playbook url")
	}
}

func TestBuildCloudInitUserDataAnsibleOnBootPull(t *testing.T) {
	onBoot := true
	playbookName := "site.yml"
	data, err := buildCloudInitUserData("", "ssh-rsa AAAA", []string{"ubuntu"}, &api.ServerAnsible{
		OnBoot: &onBoot,
		Pull: &api.ServerAnsiblePull{
			Url:          strPtr("https://github.com/example/playbooks.git"),
			PlaybookYaml: &playbookName,
		},
	})
	if err != nil {
		t.Fatalf("buildCloudInitUserData() unexpected err: %v", err)
	}
	if !strings.Contains(data, "package_update: true") {
		t.Fatalf("user-data must include package_update for ansible pull mode")
	}
	if !strings.Contains(data, "ansible:\n  install_method: distro") {
		t.Fatalf("user-data must include ansible module install_method")
	}
	if !strings.Contains(data, "package_name: ansible") {
		t.Fatalf("user-data must include ansible package_name for cloud-init module")
	}
	if !strings.Contains(data, "url: \"https://github.com/example/playbooks.git\"") {
		t.Fatalf("user-data must include ansible pull repository url")
	}
	if !strings.Contains(data, "playbook_name: \"site.yml\"") {
		t.Fatalf("user-data must include ansible pull playbook_name")
	}
	if strings.Contains(data, "\t") {
		t.Fatalf("user-data must not contain tab characters: %q", data)
	}
}

func TestBuildCloudInitUserDataAnsibleOnBootMissingRemotePlaybook(t *testing.T) {
	onBoot := true
	_, err := buildCloudInitUserData("", "ssh-rsa AAAA", []string{"ubuntu"}, &api.ServerAnsible{
		OnBoot: &onBoot,
	})
	if err == nil {
		t.Fatalf("buildCloudInitUserData() expected error")
	}
	if !strings.Contains(err.Error(), "remotePlaybook or spec.ansible.pull.url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func strPtr(v string) *string {
	return &v
}
