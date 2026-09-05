package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadKubeClientRejectsMissingFile(t *testing.T) {
	if _, err := loadKubeClient(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatalf("expected error for missing kubeconfig file")
	}
}

// writeSelfSignedCertKeyPEM は、テスト用の自己署名証明書と秘密鍵をPEMファイルとして書き出す。
func writeSelfSignedCertKeyPEM(t *testing.T, dir, name string) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	certPath = filepath.Join(dir, name+".crt")
	keyPath = filepath.Join(dir, name+".key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	return certPath, keyPath
}

func TestLoadKubeClientSupportsFilePathReferences(t *testing.T) {
	dir := t.TempDir()
	caCertPath, _ := writeSelfSignedCertKeyPEM(t, dir, "ca")
	clientCertPath, clientKeyPath := writeSelfSignedCertKeyPEM(t, dir, "client")

	path := filepath.Join(dir, "kubeconfig")
	content := fmt.Sprintf(`
clusters:
- cluster:
    certificate-authority: %s
    server: https://127.0.0.1:6443
users:
- user:
    client-certificate: %s
    client-key: %s
`, caCertPath, clientCertPath, clientKeyPath)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}

	kc, err := loadKubeClient(path)
	if err != nil {
		t.Fatalf("loadKubeClient failed for file path reference kubeconfig: %v", err)
	}
	if kc.server != "https://127.0.0.1:6443" {
		t.Fatalf("unexpected server: %s", kc.server)
	}
}

func TestLoadKubeClientRejectsInvalidBase64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `
clusters:
- cluster:
    certificate-authority-data: "not-base64!!"
    server: https://127.0.0.1:6443
users:
- user:
    client-certificate-data: ""
    client-key-data: ""
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	if _, err := loadKubeClient(path); err == nil {
		t.Fatalf("expected error for invalid certificate-authority-data")
	}
}

func TestListNodesParsesProviderIDAddressesAndTaints(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{
			"metadata":{"name":"mke-demo-node-1"},
			"spec":{"providerID":"marmot://ke-1/srv-1","taints":[{"key":"node.cloudprovider.kubernetes.io/uninitialized","effect":"NoSchedule"}]},
			"status":{"addresses":[{"type":"InternalIP","address":"172.16.1.10"},{"type":"ExternalIP","address":"192.168.1.50"}]}
		}]}`))
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() failed: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	node := nodes[0]
	if node.ProviderID != "marmot://ke-1/srv-1" {
		t.Fatalf("unexpected providerID: %q", node.ProviderID)
	}
	if node.InternalIP != "172.16.1.10" || node.ExternalIP != "192.168.1.50" {
		t.Fatalf("unexpected addresses: internal=%q external=%q", node.InternalIP, node.ExternalIP)
	}
	if len(node.Taints) != 1 || node.Taints[0].Key != "node.cloudprovider.kubernetes.io/uninitialized" {
		t.Fatalf("unexpected taints: %+v", node.Taints)
	}
}

func TestSetNodeAddressesPatchesStatusSubresource(t *testing.T) {
	var gotPath, gotContentType string
	var gotBody nodeStatusPatch
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	if err := client.SetNodeAddresses(context.Background(), "mke-demo-node-1", "172.16.1.10", "192.168.1.50"); err != nil {
		t.Fatalf("SetNodeAddresses() failed: %v", err)
	}
	if gotPath != "/api/v1/nodes/mke-demo-node-1/status" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotContentType != "application/merge-patch+json" {
		t.Fatalf("unexpected content-type: %s", gotContentType)
	}
	if len(gotBody.Status.Addresses) != 3 {
		t.Fatalf("expected 3 addresses (Hostname/InternalIP/ExternalIP), got %+v", gotBody.Status.Addresses)
	}
}

func TestPatchNodeSpecSendsProviderIDAndTaints(t *testing.T) {
	var gotPath string
	var gotBody nodeSpecPatch
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	remaining := []nodeTaint{{Key: "other", Effect: "NoSchedule"}}
	if err := client.PatchNodeSpec(context.Background(), "mke-demo-node-1", "marmot://ke-1/srv-1", remaining); err != nil {
		t.Fatalf("PatchNodeSpec() failed: %v", err)
	}
	if gotPath != "/api/v1/nodes/mke-demo-node-1" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody.Spec.ProviderID != "marmot://ke-1/srv-1" {
		t.Fatalf("unexpected providerID: %q", gotBody.Spec.ProviderID)
	}
	if len(gotBody.Spec.Taints) != 1 || gotBody.Spec.Taints[0].Key != "other" {
		t.Fatalf("unexpected taints: %+v", gotBody.Spec.Taints)
	}
}

func TestPatchNodeSpecSendsEmptyTaintsArrayWhenNil(t *testing.T) {
	var gotBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	if err := client.PatchNodeSpec(context.Background(), "mke-demo-node-1", "marmot://ke-1/srv-1", nil); err != nil {
		t.Fatalf("PatchNodeSpec() failed: %v", err)
	}
	spec, ok := gotBody["spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing spec in body: %+v", gotBody)
	}
	taints, ok := spec["taints"].([]interface{})
	if !ok {
		t.Fatalf("expected taints to be a literal array, got %+v", spec["taints"])
	}
	if len(taints) != 0 {
		t.Fatalf("expected empty taints array, got %+v", taints)
	}
}

func TestGetReturnsErrorOnNonOKStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	if _, err := client.ListNodes(context.Background()); err == nil {
		t.Fatalf("expected error on non-200 status")
	}
}

func TestListLoadBalancerServicesFiltersByTypeAndExtractsVIP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[
			{"metadata":{"name":"svc-1","namespace":"default"},"spec":{"type":"LoadBalancer"},"status":{"loadBalancer":{"ingress":[{"ip":"192.168.1.100"}]}}},
			{"metadata":{"name":"svc-2","namespace":"default"},"spec":{"type":"ClusterIP"},"status":{}}
		]}`))
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	services, err := client.ListLoadBalancerServices(context.Background())
	if err != nil {
		t.Fatalf("ListLoadBalancerServices() failed: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 LoadBalancer service, got %d: %+v", len(services), services)
	}
	if services[0].Namespace != "default" || services[0].Name != "svc-1" || services[0].VIP != "192.168.1.100" {
		t.Fatalf("unexpected service: %+v", services[0])
	}
}

func TestSetServiceLoadBalancerIngressIPPatchesStatusSubresource(t *testing.T) {
	var gotPath, gotContentType string
	var gotBody serviceStatusPatch
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	if err := client.SetServiceLoadBalancerIngressIP(context.Background(), "default", "svc-1", "192.168.1.100"); err != nil {
		t.Fatalf("SetServiceLoadBalancerIngressIP() failed: %v", err)
	}
	if gotPath != "/api/v1/namespaces/default/services/svc-1/status" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotContentType != "application/merge-patch+json" {
		t.Fatalf("unexpected content-type: %s", gotContentType)
	}
	if len(gotBody.Status.LoadBalancer.Ingress) != 1 || gotBody.Status.LoadBalancer.Ingress[0].IP != "192.168.1.100" {
		t.Fatalf("unexpected ingress: %+v", gotBody.Status.LoadBalancer.Ingress)
	}
}
