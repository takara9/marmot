package marmotd

import "testing"

func TestBuildCloudInitUserDataMultipleUsers(t *testing.T) {
	got := buildCloudInitUserData("12345", "ssh-rsa AAAA\nssh-ed25519 BBBB", []string{"root", "ubuntu"})
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
