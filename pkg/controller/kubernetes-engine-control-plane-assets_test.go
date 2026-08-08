package controller

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"strings"
	"testing"
)

func TestEnsureKubernetesEngineControlPlaneAssets(t *testing.T) {
	assets, err := EnsureKubernetesEngineControlPlaneAssets(t.TempDir(), t.TempDir(), "demo", "172.16.90.100", 6443)
	if err != nil {
		t.Fatalf("EnsureKubernetesEngineControlPlaneAssets() failed: %v", err)
	}
	certPEM, err := os.ReadFile(assets.APIServerCertPath)
	if err != nil {
		t.Fatalf("failed to read API server cert: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("failed to decode API server cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse API server cert: %v", err)
	}
	if !containsIP(cert.IPAddresses, "172.16.90.100") || !containsIP(cert.IPAddresses, "127.0.0.1") {
		t.Fatalf("API server cert IP SANs = %v", cert.IPAddresses)
	}
	if !containsString(cert.DNSNames, "kubernetes.default.svc") {
		t.Fatalf("API server cert DNS SANs = %v", cert.DNSNames)
	}

	for _, path := range []string{assets.SchedulerKubeconfigPath, assets.ControllerManagerConfigPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read kubeconfig %s: %v", path, err)
		}
		if !strings.Contains(string(data), "https://172.16.90.100:6443") {
			t.Fatalf("kubeconfig %s does not reference API server: %s", path, data)
		}
	}
	if !certFileExists(assets.ServiceAccountPublicKeyPath) || !certFileExists(assets.ServiceAccountPrivateKeyPath) {
		t.Fatalf("service-account keys were not created")
	}
}

func containsIP(values []net.IP, want string) bool {
	for _, value := range values {
		if value.String() == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
