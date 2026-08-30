package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKubeClientRejectsMissingFile(t *testing.T) {
	if _, err := loadKubeClient(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatalf("expected error for missing kubeconfig file")
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

func TestLoadKubeClientParsesServerAndRejectsEmptyClusters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `
clusters: []
users: []
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	if _, err := loadKubeClient(path); err == nil {
		t.Fatalf("expected error when clusters/users are empty")
	}
}

func TestListNodesParsesNodeNames(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"mke-demo-node-1"}},{"metadata":{"name":"mke-demo-node-2"}}]}`))
	}))
	defer ts.Close()

	client := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() failed: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Name != "mke-demo-node-1" || nodes[1].Name != "mke-demo-node-2" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

func TestListLoadBalancerServicesFiltersByType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
			{"metadata":{"name":"web","namespace":"default"},"spec":{"type":"LoadBalancer","ports":[{"name":"http","port":80,"nodePort":30080}]}},
			{"metadata":{"name":"internal","namespace":"default"},"spec":{"type":"ClusterIP","ports":[{"name":"http","port":80}]}}
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
	svc := services[0]
	if svc.Namespace != "default" || svc.Name != "web" {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if len(svc.Ports) != 1 || svc.Ports[0].NodePort != 30080 {
		t.Fatalf("unexpected ports: %+v", svc.Ports)
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
