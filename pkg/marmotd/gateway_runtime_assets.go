package marmotd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	defaultGatewayKeyDir             = "/etc/marmot/keys"
	defaultGatewayPlaybookSourceDir  = "/usr/local/marmot/gateway-playbooks"
	defaultGatewayPlaybookInstallDir = "/var/lib/marmot/ansible-playbooks/templates"
)

var (
	gatewayKeyDir             = defaultGatewayKeyDir
	gatewayPlaybookSourceDir  = defaultGatewayPlaybookSourceDir
	gatewayPlaybookInstallDir = defaultGatewayPlaybookInstallDir
)

func EnsureGatewayRuntimeAssets() error {
	if err := ensureGatewayKeyPair(); err != nil {
		return err
	}
	if err := syncGatewayPlaybookAssets(); err != nil {
		return err
	}
	return nil
}

func ensureGatewayKeyPair() error {
	if err := os.MkdirAll(gatewayKeyDir, 0o700); err != nil {
		return err
	}
	privateKeyPath := filepath.Join(gatewayKeyDir, "private.key")
	publicKeyPath := filepath.Join(gatewayKeyDir, "public.key")
	legacyPublicKeyPath := privateKeyPath + ".pub"

	// Backward compatibility: older installers created private.key.pub.
	if !fileExists(publicKeyPath) && fileExists(legacyPublicKeyPath) {
		if err := os.Rename(legacyPublicKeyPath, publicKeyPath); err != nil {
			return err
		}
	}

	privateExists := fileExists(privateKeyPath)
	publicExists := fileExists(publicKeyPath)
	if privateExists && publicExists {
		return nil
	}
	if privateExists || publicExists {
		_ = os.Remove(privateKeyPath)
		_ = os.Remove(publicKeyPath)
		_ = os.Remove(legacyPublicKeyPath)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}
	privateBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(privateBlock), 0o600); err != nil {
		return err
	}
	sshPublicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(publicKeyPath, ssh.MarshalAuthorizedKey(sshPublicKey), 0o644); err != nil {
		return err
	}
	return nil
}

func syncGatewayPlaybookAssets() error {
	if err := os.MkdirAll(gatewayPlaybookInstallDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(gatewayPlaybookSourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(gatewayPlaybookInstallDir); err != nil {
		return err
	}
	if err := os.MkdirAll(gatewayPlaybookInstallDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(gatewayPlaybookSourceDir, entry.Name())
		dst := filepath.Join(gatewayPlaybookInstallDir, entry.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(strings.TrimSpace(path))
	return err == nil
}

func GatewayPrivateKeyPath() string {
	return filepath.Join(gatewayKeyDir, "private.key")
}

func GatewayPublicKeyPath() string {
	return filepath.Join(gatewayKeyDir, "public.key")
}

func GatewayPlaybookTemplatePath() string {
	return filepath.Join(gatewayPlaybookInstallDir, "gateway-iptables.yaml.tmpl")
}

func LoadBalancerPlaybookTemplatePath() string {
	return filepath.Join(gatewayPlaybookInstallDir, "load-balancer-haproxy.yaml.tmpl")
}

func NetworkLoadBalancerPlaybookTemplatePath() string {
	return filepath.Join(gatewayPlaybookInstallDir, "network-load-balancer-iptables.yaml.tmpl")
}

func ValidateGatewayRuntimeAssets() error {
	missing := make([]string, 0, 4)
	if !fileExists(GatewayPrivateKeyPath()) {
		missing = append(missing, GatewayPrivateKeyPath())
	}
	if !fileExists(GatewayPublicKeyPath()) {
		missing = append(missing, GatewayPublicKeyPath())
	}
	if !fileExists(GatewayPlaybookTemplatePath()) {
		missing = append(missing, GatewayPlaybookTemplatePath())
	}
	if !fileExists(NetworkLoadBalancerPlaybookTemplatePath()) {
		missing = append(missing, NetworkLoadBalancerPlaybookTemplatePath())
	}
	if len(missing) > 0 {
		return fmt.Errorf("gateway runtime assets are missing: %s", strings.Join(missing, ", "))
	}
	return nil
}
