package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultKubernetesEnginePkiDir はクラスタ専用CA・証明書を配置するディレクトリの親。
	// 実際のファイルは "<DefaultKubernetesEnginePkiDir>/<cluster>" 配下に置かれる。
	DefaultKubernetesEnginePkiDir = "/var/lib/marmot/mke/pki"

	kubernetesEngineCAValidity   = 10 * 365 * 24 * time.Hour
	kubernetesEngineCertValidity = 365 * 24 * time.Hour
	kubernetesEngineCAKeyBits    = 4096
	kubernetesEngineCertKeyBits  = 2048
)

// KubernetesEngineCertUsage は発行する証明書の用途(サーバー/クライアント)を表す。
type KubernetesEngineCertUsage int

const (
	KubernetesEngineCertUsageServer KubernetesEngineCertUsage = iota
	KubernetesEngineCertUsageClient
)

// KubernetesEngineCertRequest は証明書発行に必要なパラメータ。
type KubernetesEngineCertRequest struct {
	// Name はファイル名の基点(例: "kube-apiserver", "kubelet-node1")。
	Name        string
	CommonName  string
	Usage       KubernetesEngineCertUsage
	DNSNames    []string
	IPAddresses []net.IP
}

func kubernetesEngineClusterPkiDir(pkiDir, clusterName string) string {
	return filepath.Join(pkiDir, clusterName)
}

// KubernetesEngineCAPaths はクラスタ専用CAの証明書・秘密鍵のパスを返す。
func KubernetesEngineCAPaths(pkiDir, clusterName string) (certPath, keyPath string) {
	dir := kubernetesEngineClusterPkiDir(pkiDir, clusterName)
	return filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
}

// KubernetesEngineCertPaths は発行済み証明書・秘密鍵のパスを返す。
func KubernetesEngineCertPaths(pkiDir, clusterName, name string) (certPath, keyPath string) {
	dir := kubernetesEngineClusterPkiDir(pkiDir, clusterName)
	return filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
}

// validateKubernetesEnginePkiClusterName はクラスタ名がディレクトリ/ファイル名として
// 安全であることを検証し、前後の空白を除いた名前を返す。
func validateKubernetesEnginePkiClusterName(clusterName string) (string, error) {
	name := strings.TrimSpace(clusterName)
	if name == "" {
		return "", fmt.Errorf("cluster name is empty")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("cluster name is invalid: %q", clusterName)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", fmt.Errorf("cluster name contains invalid character %q", r)
	}
	return name, nil
}

// EnsureKubernetesEngineCA はクラスタ専用の自己署名CA(証明書・秘密鍵)を生成する。
// 既に生成済みの場合は再生成せずそのパスを返す(冪等)。
func EnsureKubernetesEngineCA(pkiDir, clusterName string) (certPath, keyPath string, err error) {
	name, err := validateKubernetesEnginePkiClusterName(clusterName)
	if err != nil {
		return "", "", err
	}
	certPath, keyPath = KubernetesEngineCAPaths(pkiDir, name)
	if certFileExists(certPath) && certFileExists(keyPath) {
		return certPath, keyPath, nil
	}

	dir := kubernetesEngineClusterPkiDir(pkiDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("failed to create pki dir: %w", err)
	}

	caKey, err := rsa.GenerateKey(rand.Reader, kubernetesEngineCAKeyBits)
	if err != nil {
		return "", "", err
	}
	serial, err := newCertificateSerialNumber()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("mke-%s-ca", name),
			Organization: []string{"marmot"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(kubernetesEngineCAValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}
	if err := writePemFile(certPath, "CERTIFICATE", caDER, 0o644); err != nil {
		return "", "", err
	}
	if err := writePemFile(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(caKey), 0o600); err != nil {
		_ = os.Remove(certPath)
		return "", "", err
	}
	return certPath, keyPath, nil
}

// IssueKubernetesEngineCertificate はクラスタ専用CAで署名したリーフ証明書を発行する。
// 既に同名の証明書が存在する場合は再発行せずそのパスを返す(冪等)。CAが未生成の場合はエラーになる。
func IssueKubernetesEngineCertificate(pkiDir, clusterName string, req KubernetesEngineCertRequest) (certPath, keyPath string, err error) {
	name, err := validateKubernetesEnginePkiClusterName(clusterName)
	if err != nil {
		return "", "", err
	}
	reqName := strings.TrimSpace(req.Name)
	if reqName == "" {
		return "", "", fmt.Errorf("certificate request name is empty")
	}
	if reqName != filepath.Base(reqName) || reqName == "." || reqName == ".." || strings.Contains(reqName, "\\") {
		return "", "", fmt.Errorf("certificate request name is invalid: %q", req.Name)
	}
	for _, r := range reqName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", "", fmt.Errorf("certificate request name contains invalid character %q", r)
	}
	certPath, keyPath = KubernetesEngineCertPaths(pkiDir, name, reqName)
	if certFileExists(certPath) && certFileExists(keyPath) {
		return certPath, keyPath, nil
	}

	caCertPath, caKeyPath := KubernetesEngineCAPaths(pkiDir, name)
	caCert, caKey, err := loadKubernetesEngineCA(caCertPath, caKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to load cluster CA: %w", err)
	}

	var extKeyUsage []x509.ExtKeyUsage
	switch req.Usage {
	case KubernetesEngineCertUsageServer:
		extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	case KubernetesEngineCertUsageClient:
		extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	default:
		return "", "", fmt.Errorf("unknown certificate usage: %v", req.Usage)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, kubernetesEngineCertKeyBits)
	if err != nil {
		return "", "", err
	}
	serial, err := newCertificateSerialNumber()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	leafTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   req.CommonName,
			Organization: []string{"marmot"},
		},
		NotBefore:   now.Add(-time.Hour),
		NotAfter:    now.Add(kubernetesEngineCertValidity),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: extKeyUsage,
		DNSNames:    req.DNSNames,
		IPAddresses: req.IPAddresses,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}

	dir := kubernetesEngineClusterPkiDir(pkiDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	if err := writePemFile(certPath, "CERTIFICATE", leafDER, 0o644); err != nil {
		return "", "", err
	}
	if err := writePemFile(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(leafKey), 0o600); err != nil {
		_ = os.Remove(certPath)
		return "", "", err
	}
	return certPath, keyPath, nil
}

func newCertificateSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func writePemFile(path, blockType string, der []byte, mode os.FileMode) error {
	block := &pem.Block{Type: blockType, Bytes: der}
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(pem.EncodeToMemory(block)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

func loadKubernetesEngineCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA certificate PEM: %s", certPath)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA private key PEM: %s", keyPath)
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func certFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
