package marmotd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/pkg/db"
)

// setupGatewayKeyDir は、テストごとに独立したキーディレクトリを用意し、
// EnsureGatewayRuntimeAssetsでローカル鍵ペアを生成する。
func setupGatewayKeyDir(t *testing.T) {
	t.Helper()
	oldKeyDir := gatewayKeyDir
	t.Cleanup(func() { gatewayKeyDir = oldKeyDir })
	gatewayKeyDir = filepath.Join(t.TempDir(), "keys")
	if err := ensureGatewayKeyPair(); err != nil {
		t.Fatalf("ensureGatewayKeyPair() failed: %v", err)
	}
}

func TestSyncGatewayKeyPairWithCluster_PublishesLocalKeyWhenNoneShared(t *testing.T) {
	setupGatewayKeyDir(t)
	before, err := os.ReadFile(GatewayPrivateKeyPath())
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}

	database := newGatewayKeySyncTestDatabase(t)
	if err := SyncGatewayKeyPairWithCluster(database); err != nil {
		t.Fatalf("SyncGatewayKeyPairWithCluster() failed: %v", err)
	}

	after, err := os.ReadFile(GatewayPrivateKeyPath())
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("local key should stay unchanged when it becomes the cluster-shared key")
	}

	var shared gatewayKeyPairRecord
	if _, err := database.GetJSON(gatewayKeyPairEtcdKey, &shared); err != nil {
		t.Fatalf("GetJSON() failed after publish: %v", err)
	}
	if shared.PrivateKeyPEM != string(after) {
		t.Fatalf("etcd shared key does not match published local key")
	}
}

func TestSyncGatewayKeyPairWithCluster_AdoptsExistingSharedKey(t *testing.T) {
	setupGatewayKeyDir(t)
	database := newGatewayKeySyncTestDatabase(t)

	// 別ホストが先にpublish済みの状態を模倣する。
	otherHostRecord := gatewayKeyPairRecord{
		PrivateKeyPEM:       "-----BEGIN RSA PRIVATE KEY-----\nother-host-key\n-----END RSA PRIVATE KEY-----\n",
		PublicKeyAuthorized: "ssh-rsa OTHERHOSTKEY marmot-gateway\n",
	}
	if err := database.PutJSONCAS(gatewayKeyPairEtcdKey, 0, otherHostRecord); err != nil {
		t.Fatalf("PutJSONCAS() failed to seed shared key: %v", err)
	}

	if err := SyncGatewayKeyPairWithCluster(database); err != nil {
		t.Fatalf("SyncGatewayKeyPairWithCluster() failed: %v", err)
	}

	gotPrivate, err := os.ReadFile(GatewayPrivateKeyPath())
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	gotPublic, err := os.ReadFile(GatewayPublicKeyPath())
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	if string(gotPrivate) != otherHostRecord.PrivateKeyPEM {
		t.Fatalf("local private key was not converged to the cluster-shared key")
	}
	if string(gotPublic) != otherHostRecord.PublicKeyAuthorized {
		t.Fatalf("local public key was not converged to the cluster-shared key")
	}
}

func newGatewayKeySyncTestDatabase(t *testing.T) *db.Database {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("MARMOT_TEST_ETCD_ENDPOINT"))
	if endpoint == "" {
		endpoint = startGatewayKeySyncTestEtcdContainer(t)
	}

	database, err := db.NewDatabase(endpoint)
	if err != nil {
		t.Fatalf("NewDatabase(%q) failed: %v", endpoint, err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database
}

func startGatewayKeySyncTestEtcdContainer(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker command not found; set MARMOT_TEST_ETCD_ENDPOINT to run gateway key sync tests without docker")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() failed while reserving test port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	portMapping := fmt.Sprintf("%d:2379", port)
	cmd := exec.Command("docker", "run", "-d", "--rm", "-p", portMapping, "ghcr.io/takara9/etcd:3.6.5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("failed to start etcd test container: %v, output=%s", err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}
	t.Cleanup(func() {
		stopCmd := exec.Command("docker", "stop", containerID)
		_, _ = stopCmd.CombinedOutput()
	})

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		database, err := db.NewDatabase(endpoint)
		if err == nil {
			_ = database.Close()
			return endpoint
		}
		time.Sleep(300 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for etcd container at %s", endpoint)
	return ""
}
