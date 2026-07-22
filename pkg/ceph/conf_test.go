package ceph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseConnectionFromConf(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "ceph.conf")
	keyringPath := filepath.Join(dir, "ceph.client.admin.keyring")

	conf := "[global]\nmon_host = 10.1.4.11:6789,10.1.4.12:6789\nname = client.admin\n"
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	if err := os.WriteFile(keyringPath, []byte("[client.admin]\n\tkey = dummy\n"), 0o600); err != nil {
		t.Fatalf("write keyring: %v", err)
	}

	monitors, user, err := ParseConnectionFromConf(confPath, keyringPath)
	if err != nil {
		t.Fatalf("ParseConnectionFromConf: %v", err)
	}

	wantMonitors := []string{"10.1.4.11:6789", "10.1.4.12:6789"}
	if !reflect.DeepEqual(monitors, wantMonitors) {
		t.Fatalf("monitors mismatch: got=%v want=%v", monitors, wantMonitors)
	}
	if user != "admin" {
		t.Fatalf("user mismatch: got=%q want=%q", user, "admin")
	}
}

func TestParseConnectionFromConfFallbackUserFromKeyring(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "ceph.conf")
	keyringPath := filepath.Join(dir, "ceph.client.ubuntu.keyring")

	conf := "[global]\nmon_host = 10.1.4.11:6789\n"
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	if err := os.WriteFile(keyringPath, []byte("[client.ubuntu]\n\tkey = dummy\n"), 0o600); err != nil {
		t.Fatalf("write keyring: %v", err)
	}

	_, user, err := ParseConnectionFromConf(confPath, keyringPath)
	if err != nil {
		t.Fatalf("ParseConnectionFromConf: %v", err)
	}
	if user != "ubuntu" {
		t.Fatalf("user mismatch: got=%q want=%q", user, "ubuntu")
	}
}

func TestParseConnectionFromConfRequiresMonitors(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "ceph.conf")
	keyringPath := filepath.Join(dir, "ceph.client.admin.keyring")

	if err := os.WriteFile(confPath, []byte("[global]\nname = client.admin\n"), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	if err := os.WriteFile(keyringPath, []byte("[client.admin]\n\tkey = dummy\n"), 0o600); err != nil {
		t.Fatalf("write keyring: %v", err)
	}

	_, _, err := ParseConnectionFromConf(confPath, keyringPath)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}
