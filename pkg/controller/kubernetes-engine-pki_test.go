package controller

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureKubernetesEngineCACreatesAndIsIdempotent(t *testing.T) {
	pkiDir := t.TempDir()

	certPath, keyPath, err := EnsureKubernetesEngineCA(pkiDir, "demo")
	if err != nil {
		t.Fatalf("EnsureKubernetesEngineCA() failed: %v", err)
	}
	if !certFileExists(certPath) || !certFileExists(keyPath) {
		t.Fatalf("CA cert/key were not created: certPath=%s keyPath=%s", certPath, keyPath)
	}

	firstCert, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read CA cert: %v", err)
	}

	// 2回目の呼び出しは再生成せず同じ内容を返す(冪等)こと
	certPath2, keyPath2, err := EnsureKubernetesEngineCA(pkiDir, "demo")
	if err != nil {
		t.Fatalf("EnsureKubernetesEngineCA() second call failed: %v", err)
	}
	if certPath2 != certPath || keyPath2 != keyPath {
		t.Fatalf("paths changed between calls: (%s,%s) vs (%s,%s)", certPath, keyPath, certPath2, keyPath2)
	}
	secondCert, err := os.ReadFile(certPath2)
	if err != nil {
		t.Fatalf("failed to read CA cert: %v", err)
	}
	if string(firstCert) != string(secondCert) {
		t.Fatalf("CA certificate was regenerated on second call")
	}
}

func TestEnsureKubernetesEngineCARejectsInvalidClusterName(t *testing.T) {
	pkiDir := t.TempDir()
	if _, _, err := EnsureKubernetesEngineCA(pkiDir, " "); err == nil {
		t.Fatalf("expected error for empty cluster name, got nil")
	}
	if _, _, err := EnsureKubernetesEngineCA(pkiDir, "../etc"); err == nil {
		t.Fatalf("expected error for invalid cluster name, got nil")
	}
}

func TestIssueKubernetesEngineCertificateServerAndClient(t *testing.T) {
	pkiDir := t.TempDir()
	caCertPath, _, err := EnsureKubernetesEngineCA(pkiDir, "demo")
	if err != nil {
		t.Fatalf("EnsureKubernetesEngineCA() failed: %v", err)
	}
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		t.Fatalf("failed to read CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatalf("failed to load CA cert into pool")
	}

	serverCertPath, serverKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
		Name:        "kube-apiserver",
		CommonName:  "kube-apiserver",
		Usage:       KubernetesEngineCertUsageServer,
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	if err != nil {
		t.Fatalf("IssueKubernetesEngineCertificate(server) failed: %v", err)
	}
	if !certFileExists(serverCertPath) || !certFileExists(serverKeyPath) {
		t.Fatalf("server cert/key were not created")
	}
	assertCertVerifiesAgainstCA(t, caPool, serverCertPath, x509.ExtKeyUsageServerAuth)

	clientCertPath, clientKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
		Name:       "kubelet-node1",
		CommonName: "system:node:node1",
		Usage:      KubernetesEngineCertUsageClient,
	})
	if err != nil {
		t.Fatalf("IssueKubernetesEngineCertificate(client) failed: %v", err)
	}
	if !certFileExists(clientCertPath) || !certFileExists(clientKeyPath) {
		t.Fatalf("client cert/key were not created")
	}
	assertCertVerifiesAgainstCA(t, caPool, clientCertPath, x509.ExtKeyUsageClientAuth)

	// 同名での再発行は既存ファイルを再利用する(冪等)こと
	firstCert, err := os.ReadFile(serverCertPath)
	if err != nil {
		t.Fatalf("failed to read server cert: %v", err)
	}
	serverCertPath2, serverKeyPath2, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
		Name:       "kube-apiserver",
		CommonName: "kube-apiserver",
		Usage:      KubernetesEngineCertUsageServer,
	})
	if err != nil {
		t.Fatalf("IssueKubernetesEngineCertificate() second call failed: %v", err)
	}
	if serverCertPath2 != serverCertPath || serverKeyPath2 != serverKeyPath {
		t.Fatalf("paths changed between calls")
	}
	secondCert, err := os.ReadFile(serverCertPath2)
	if err != nil {
		t.Fatalf("failed to read server cert: %v", err)
	}
	if string(firstCert) != string(secondCert) {
		t.Fatalf("server certificate was reissued on second call")
	}
}

func TestIssueKubernetesEngineCertificateRequiresExistingCA(t *testing.T) {
	pkiDir := t.TempDir()
	if _, _, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
		Name:       "kube-apiserver",
		CommonName: "kube-apiserver",
		Usage:      KubernetesEngineCertUsageServer,
	}); err == nil {
		t.Fatalf("expected error when CA does not exist, got nil")
	}
}

func assertCertVerifiesAgainstCA(t *testing.T, caPool *x509.CertPool, certPath string, usage x509.ExtKeyUsage) {
	t.Helper()
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read cert %s: %v", certPath, err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("failed to parse PEM block for %s", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate %s: %v", certPath, err)
	}
	opts := x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{usage},
	}
	if _, err := cert.Verify(opts); err != nil {
		t.Fatalf("certificate %s did not verify against CA: %v", certPath, err)
	}
}

func TestKubernetesEnginePkiDirLayout(t *testing.T) {
	pkiDir := t.TempDir()
	certPath, keyPath, err := EnsureKubernetesEngineCA(pkiDir, "demo")
	if err != nil {
		t.Fatalf("EnsureKubernetesEngineCA() failed: %v", err)
	}
	wantDir := filepath.Join(pkiDir, "demo")
	if filepath.Dir(certPath) != wantDir || filepath.Dir(keyPath) != wantDir {
		t.Fatalf("CA files not under expected dir %s: certPath=%s keyPath=%s", wantDir, certPath, keyPath)
	}
}
