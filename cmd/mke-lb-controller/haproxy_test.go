package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHAProxyConfigSkipsServicesWithoutVIP(t *testing.T) {
	nodes := []nodeInfo{{Name: "mke-demo-node-1", InternalIP: "172.16.1.10"}}
	services := []loadBalancerServiceInfo{
		{Namespace: "default", Name: "no-vip", Ports: []servicePortInfo{{Port: 80, NodePort: 30080}}},
	}
	config := renderHAProxyConfig(nodes, services)
	if strings.Contains(config, "no-vip") {
		t.Fatalf("expected service without VIP to be excluded, got:\n%s", config)
	}
}

func TestRenderHAProxyConfigIncludesFrontendAndBackend(t *testing.T) {
	nodes := []nodeInfo{
		{Name: "mke-demo-node-1", InternalIP: "172.16.1.10"},
		{Name: "mke-demo-node-2", InternalIP: "172.16.1.11"},
	}
	services := []loadBalancerServiceInfo{
		{Namespace: "web", Name: "app", VIP: "192.168.1.200", Ports: []servicePortInfo{{Port: 80, NodePort: 30080}}},
	}
	config := renderHAProxyConfig(nodes, services)

	if !strings.Contains(config, "frontend fe_web_app_80") {
		t.Fatalf("expected frontend block, got:\n%s", config)
	}
	if !strings.Contains(config, "bind 192.168.1.200:80") {
		t.Fatalf("expected VIP bind, got:\n%s", config)
	}
	if !strings.Contains(config, "backend be_web_app_80") {
		t.Fatalf("expected backend block, got:\n%s", config)
	}
	if !strings.Contains(config, "server mke-demo-node-1 172.16.1.10:30080 check") {
		t.Fatalf("expected node-1 backend server line, got:\n%s", config)
	}
	if !strings.Contains(config, "server mke-demo-node-2 172.16.1.11:30080 check") {
		t.Fatalf("expected node-2 backend server line, got:\n%s", config)
	}
}

func TestApplyHAProxyConfigSkipsWhenHashMatchesWithoutTouchingHAProxy(t *testing.T) {
	content := "global\n"
	sum := sha256.Sum256([]byte(content))
	matchingHash := hex.EncodeToString(sum[:])

	hash, changed, err := applyHAProxyConfig(filepath.Join(t.TempDir(), "haproxy.cfg"), content, matchingHash)
	if err != nil {
		t.Fatalf("applyHAProxyConfig() failed: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false when hash already matches")
	}
	if hash != matchingHash {
		t.Fatalf("expected returned hash to equal lastAppliedHash, got %s", hash)
	}
}

func TestApplyHAProxyConfigAppliesAndSkipsWhenUnchanged(t *testing.T) {
	if _, err := exec.LookPath("haproxy"); err != nil {
		t.Skip("haproxy binary not found; skipping validation test")
	}
	content := "global\n"
	hash, changed, err := applyHAProxyConfig(filepath.Join(t.TempDir(), "haproxy.cfg"), content, "")
	if err != nil {
		t.Fatalf("applyHAProxyConfig() failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected first apply to report changed=true")
	}

	_, changedAgain, err := applyHAProxyConfig(filepath.Join(t.TempDir(), "haproxy.cfg"), content, hash)
	if err != nil {
		t.Fatalf("applyHAProxyConfig() second call failed: %v", err)
	}
	if changedAgain {
		t.Fatalf("expected second apply with unchanged content to report changed=false")
	}
}

func TestApplyHAProxyConfigValidatesWithHAProxyBinary(t *testing.T) {
	if _, err := exec.LookPath("haproxy"); err != nil {
		t.Skip("haproxy binary not found; skipping validation test")
	}
	path := filepath.Join(t.TempDir(), "haproxy.cfg")
	if _, _, err := applyHAProxyConfig(path, "this is not a valid haproxy config", ""); err == nil {
		t.Fatalf("expected validation error for invalid HAProxy config")
	}
}
