package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPreservedNetplanEthernetsIgnoresBackupFiles(t *testing.T) {
	root := t.TempDir()
	netplanDir := filepath.Join(root, "etc", "netplan")
	if err := os.MkdirAll(netplanDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(netplanDir, "00-nic.yaml"), []byte(`network:
  version: 2
  renderer: networkd
  ethernets:
    enp1s0:
      dhcp4: false
      dhcp6: false
    enp2s0:
      addresses:
        - 172.16.0.204/16
      dhcp4: false
      dhcp6: false
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netplanDir, "00-nic.yaml.marmot.bak"), []byte(`network:
  version: 2
  ethernets:
    enp9s0:
      dhcp4: false
      dhcp6: false
	`), 0644); err != nil {
		t.Fatal(err)
	}
	preserved, err := loadPreservedNetplanEthernets(netplanDir, "enp1s0")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := preserved["enp2s0"]; !ok {
		t.Fatalf("expected enp2s0 to be preserved, got %#v", preserved)
	}
	if _, ok := preserved["enp1s0"]; ok {
		t.Fatalf("expected enp1s0 to be excluded, got %#v", preserved)
	}
	if _, ok := preserved["enp9s0"]; ok {
		t.Fatalf("expected backup file to be ignored, got %#v", preserved)
	}
}

func TestCreateNetplanConfigPreservesAdditionalEthernets(t *testing.T) {
	root := t.TempDir()
	netplanDir := filepath.Join(root, "etc", "netplan")
	if err := os.MkdirAll(netplanDir, 0755); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(netplanDir, "00-host-bridge.yaml")
	preserved := map[string]Ethernet{
		"enp2s0": {
			Addresses: []string{"172.16.0.204/16"},
			DHCP4:     false,
			DHCP6:     false,
			Routes: []Route{{
				To:  "10.0.0.0/8",
				Via: "172.16.0.1",
			}},
		},
	}

	if err := createNetplanConfig(filePath, "enp1s0", bridgeConfigInfo{
		IPAddress: "192.168.1.204",
		Gateway:   "192.168.1.1",
		Domain:    "labo.local",
	}, preserved); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "br0:") {
		t.Fatalf("expected br0 in generated config: %s", content)
	}
	if !strings.Contains(content, "enp1s0:") {
		t.Fatalf("expected enp1s0 in generated config: %s", content)
	}
	if !strings.Contains(content, "enp2s0:") {
		t.Fatalf("expected enp2s0 in generated config: %s", content)
	}
	if !strings.Contains(content, "172.16.0.204/16") {
		t.Fatalf("expected preserved address in generated config: %s", content)
	}
	if !strings.Contains(content, "192.168.1.204") {
		t.Fatalf("expected bridge address in generated config: %s", content)
	}
	if strings.Contains(content, ".marmot.bak") {
		t.Fatalf("backup suffix should not appear in active config: %s", content)
	}
}
