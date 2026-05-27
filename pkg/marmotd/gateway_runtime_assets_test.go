package marmotd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGatewayRuntimeAssets_CreatesKeysAndSyncsPlaybooks(t *testing.T) {
	oldKeyDir := gatewayKeyDir
	oldSourceDir := gatewayPlaybookSourceDir
	oldInstallDir := gatewayPlaybookInstallDir
	t.Cleanup(func() {
		gatewayKeyDir = oldKeyDir
		gatewayPlaybookSourceDir = oldSourceDir
		gatewayPlaybookInstallDir = oldInstallDir
	})

	tempDir := t.TempDir()
	gatewayKeyDir = filepath.Join(tempDir, "keys")
	gatewayPlaybookSourceDir = filepath.Join(tempDir, "src-playbooks")
	gatewayPlaybookInstallDir = filepath.Join(tempDir, "dst-playbooks")

	if err := os.MkdirAll(gatewayPlaybookSourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gatewayPlaybookSourceDir, "gateway-iptables.yaml.tmpl"), []byte("template-body"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed for source template: %v", err)
	}
	if err := os.MkdirAll(gatewayPlaybookInstallDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() failed for install dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gatewayPlaybookInstallDir, "stale-file"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed for stale template: %v", err)
	}

	if err := EnsureGatewayRuntimeAssets(); err != nil {
		t.Fatalf("EnsureGatewayRuntimeAssets() failed: %v", err)
	}
	if err := ValidateGatewayRuntimeAssets(); err != nil {
		t.Fatalf("ValidateGatewayRuntimeAssets() failed: %v", err)
	}

	publicKeyBytes, err := os.ReadFile(GatewayPublicKeyPath())
	if err != nil {
		t.Fatalf("ReadFile() failed for public key: %v", err)
	}
	if !strings.HasPrefix(string(publicKeyBytes), "ssh-rsa ") {
		t.Fatalf("unexpected public key prefix: %q", string(publicKeyBytes))
	}
	if _, err := os.Stat(filepath.Join(gatewayPlaybookInstallDir, "stale-file")); !os.IsNotExist(err) {
		t.Fatalf("stale playbook file should be removed, stat err=%v", err)
	}
	templateBytes, err := os.ReadFile(GatewayPlaybookTemplatePath())
	if err != nil {
		t.Fatalf("ReadFile() failed for installed template: %v", err)
	}
	if string(templateBytes) != "template-body" {
		t.Fatalf("unexpected installed template: %q", string(templateBytes))
	}
}
