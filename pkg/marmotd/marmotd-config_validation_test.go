package marmotd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/takara9/marmot/pkg/marmotd"
)

func TestLoadConfig_HostBridgeAutoIPSettingsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marmotd.json")
	content := []byte(`{
		"host-bridge-ip-net-addr":"192.168.1.0/24",
		"host-bridge-ip-addr-start":"192.168.1.129",
		"host-bridge-ip-addr-end":"192.168.1.190",
		"host-bridge-default":{
			"netmasklen":24,
			"nameservers":{"addresses":["8.8.8.8"],"search":["labo.local"]},
			"routes":[{"to":"default","via":"192.168.1.1"}]
		}
	}`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	cfg, err := marmotd.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}
	if cfg.HostBridgeIPNetAddr != "192.168.1.0/24" {
		t.Fatalf("HostBridgeIPNetAddr = %q, want %q", cfg.HostBridgeIPNetAddr, "192.168.1.0/24")
	}
	if cfg.HostBridgeDefault == nil || cfg.HostBridgeDefault.Routes == nil || len(*cfg.HostBridgeDefault.Routes) != 1 {
		t.Fatalf("HostBridgeDefault.Routes = %v, want 1 route", cfg.HostBridgeDefault)
	}
}

func TestLoadConfig_HostBridgeAutoIPSettingsRequireAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marmotd.json")
	content := []byte(`{
		"host-bridge-ip-net-addr":"192.168.1.0/24",
		"host-bridge-ip-addr-start":"192.168.1.129"
	}`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	if _, err := marmotd.LoadConfig(path); err == nil {
		t.Fatalf("LoadConfig() expected error, got nil")
	}
}
