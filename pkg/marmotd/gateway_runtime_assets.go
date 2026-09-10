package marmotd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takara9/marmot/pkg/db"
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

// gatewayKeyPairEtcdKey は、marmotクラスタの全ホストで共有するゲートウェイSSH鍵ペアを
// 格納するetcdキー。
const gatewayKeyPairEtcdKey = "/marmot/system/gateway-keypair"

// gatewayKeyPairRecord はetcdに保存する鍵ペアの内容。
type gatewayKeyPairRecord struct {
	PrivateKeyPEM       string `json:"privateKeyPem"`
	PublicKeyAuthorized string `json:"publicKeyAuthorized"`
}

// SyncGatewayKeyPairWithCluster は、ローカルのゲートウェイSSH鍵ペアをetcd上のクラスタ共有値と
// 突き合わせ、ホスト間で一致させる。marmotクラスタ(複数ホスト)構成では、ホストごとに
// ensureGatewayKeyPairでローカル生成した鍵がホスト間で食い違うと、あるホストが作成した
// VMの authorized_keys に埋め込まれた公開鍵と、別ホストが後から使う秘密鍵が一致せず、
// SSHによるノードプロビジョニングが恒久的に失敗する(単一ホスト構成では発生しない)。
// この関数は、etcdにまだ共有鍵が無ければローカル鍵をCASで publish し、既にあれば
// ローカルのファイルをその内容で上書きしてクラスタ全体を1つの鍵ペアへ収束させる。
func SyncGatewayKeyPairWithCluster(database *db.Database) error {
	if database == nil {
		return fmt.Errorf("database is required to sync gateway key pair")
	}
	privateKeyPath := GatewayPrivateKeyPath()
	publicKeyPath := GatewayPublicKeyPath()
	localPrivate, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read local gateway private key: %w", err)
	}
	localPublic, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read local gateway public key: %w", err)
	}

	var shared gatewayKeyPairRecord
	_, err = database.GetJSON(gatewayKeyPairEtcdKey, &shared)
	switch {
	case err == nil:
		// クラスタ共有鍵が既に存在する場合、ローカルと異なれば上書きして収束させる。
		return applyGatewaySharedKeyPairIfDifferent(shared, string(localPrivate), string(localPublic), privateKeyPath, publicKeyPath)
	case errors.Is(err, db.ErrNotFound):
		// 誰も共有鍵を publish していない場合、ローカル鍵を最初の共有鍵としてCAS登録を試みる。
		local := gatewayKeyPairRecord{PrivateKeyPEM: string(localPrivate), PublicKeyAuthorized: string(localPublic)}
		putErr := database.PutJSONCAS(gatewayKeyPairEtcdKey, 0, local)
		if putErr == nil {
			return nil
		}
		if !errors.Is(putErr, db.ErrUpdateConflict) {
			return fmt.Errorf("failed to publish gateway key pair to etcd: %w", putErr)
		}
		// 他ホストが先にpublishしていた場合、その値を取得して収束させる。
		if _, getErr := database.GetJSON(gatewayKeyPairEtcdKey, &shared); getErr != nil {
			return fmt.Errorf("failed to fetch gateway key pair after publish conflict: %w", getErr)
		}
		return applyGatewaySharedKeyPairIfDifferent(shared, string(localPrivate), string(localPublic), privateKeyPath, publicKeyPath)
	default:
		return fmt.Errorf("failed to fetch cluster gateway key pair: %w", err)
	}
}

func applyGatewaySharedKeyPairIfDifferent(shared gatewayKeyPairRecord, localPrivate, localPublic, privateKeyPath, publicKeyPath string) error {
	if shared.PrivateKeyPEM == localPrivate && shared.PublicKeyAuthorized == localPublic {
		return nil
	}
	if err := os.WriteFile(privateKeyPath, []byte(shared.PrivateKeyPEM), 0o600); err != nil {
		return fmt.Errorf("failed to apply cluster gateway private key: %w", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte(shared.PublicKeyAuthorized), 0o644); err != nil {
		return fmt.Errorf("failed to apply cluster gateway public key: %w", err)
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

func VpnGatewayPlaybookTemplatePath() string {
	return filepath.Join(gatewayPlaybookInstallDir, "vpn-gateway-openvpn.yaml.tmpl")
}

func LoadBalancerPlaybookTemplatePath() string {
	return filepath.Join(gatewayPlaybookInstallDir, "load-balancer-haproxy.yaml.tmpl")
}

func NetworkLoadBalancerPlaybookTemplatePath() string {
	return filepath.Join(gatewayPlaybookInstallDir, "network-load-balancer-iptables.yaml.tmpl")
}

func ValidateGatewayRuntimeAssets() error {
	missing := make([]string, 0, 3)
	if !fileExists(GatewayPrivateKeyPath()) {
		missing = append(missing, GatewayPrivateKeyPath())
	}
	if !fileExists(GatewayPublicKeyPath()) {
		missing = append(missing, GatewayPublicKeyPath())
	}
	if !fileExists(GatewayPlaybookTemplatePath()) {
		missing = append(missing, GatewayPlaybookTemplatePath())
	}
	if len(missing) > 0 {
		return fmt.Errorf("gateway runtime assets are missing: %s", strings.Join(missing, ", "))
	}
	return nil
}
