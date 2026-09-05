package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// newReconcileTestServers は、1ノード(host-bridgeアドレスがExternalIPと異なる)を持つ
// kube-apiserver/marmotdの最小限のフェイクを組み立てる。setNodeAddressesCalledは
// PATCH /api/v1/nodes/<name>/status が呼ばれたかどうかを示す。
func newReconcileTestServers(t *testing.T) (kube *httptest.Server, marmotd *httptest.Server, setNodeAddressesCalled *atomic.Bool) {
	t.Helper()
	setNodeAddressesCalled = &atomic.Bool{}

	kube = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"mke-demo-node-1"},"status":{"addresses":[]}}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/nodes/mke-demo-node-1/status":
			setNodeAddressesCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(kube.Close)

	marmotd = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server" {
			t.Fatalf("unexpected marmotd request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"metadata":{"name":"mke-demo-node-1"},"spec":{"networkInterface":[{"networkname":"host-bridge","address":"192.168.1.50"}]}}]`))
	}))
	t.Cleanup(marmotd.Close)

	return kube, marmotd, setNodeAddressesCalled
}

func TestReconcileCallsSetNodeAddressesWhenCloudControllerManagerDisabled(t *testing.T) {
	kube, marmotd, setNodeAddressesCalled := newReconcileTestServers(t)

	client := &kubeClient{httpClient: kube.Client(), server: kube.URL}
	mClient := &marmotdClient{httpClient: marmotd.Client(), baseURL: marmotd.URL + "/api/v1", apiKey: "test-token"}
	haproxyConfigPath := filepath.Join(t.TempDir(), "haproxy.cfg")

	reconcile(context.Background(), client, mClient, haproxyConfigPath, "", "", map[string]string{}, false)

	if !setNodeAddressesCalled.Load() {
		t.Fatalf("expected SetNodeAddresses to be called when cloud-controller-manager is disabled")
	}
}

func TestReconcileSkipsSetNodeAddressesWhenCloudControllerManagerEnabled(t *testing.T) {
	kube, marmotd, setNodeAddressesCalled := newReconcileTestServers(t)

	client := &kubeClient{httpClient: kube.Client(), server: kube.URL}
	mClient := &marmotdClient{httpClient: marmotd.Client(), baseURL: marmotd.URL + "/api/v1", apiKey: "test-token"}
	haproxyConfigPath := filepath.Join(t.TempDir(), "haproxy.cfg")

	reconcile(context.Background(), client, mClient, haproxyConfigPath, "", "", map[string]string{}, true)

	if setNodeAddressesCalled.Load() {
		t.Fatalf("expected SetNodeAddresses NOT to be called when cloud-controller-manager is enabled")
	}
}

// newReconcileVipTestServers は、VIP未払い出しのLoadBalancer Serviceを1件持つ
// kube-apiserver/marmotdの最小限のフェイクを組み立てる。requestVipCalled/releaseVipCalledは、
// それぞれのVIP払い出し/解放エンドポイントが呼ばれたかどうかを示す。
func newReconcileVipTestServers(t *testing.T, knownServiceVIPs map[string]string) (kube *httptest.Server, marmotd *httptest.Server, requestVipCalled, releaseVipCalled *atomic.Bool) {
	t.Helper()
	requestVipCalled = &atomic.Bool{}
	releaseVipCalled = &atomic.Bool{}
	hasExistingKnownVIP := len(knownServiceVIPs) > 0

	kube = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			if hasExistingKnownVIP {
				_, _ = w.Write([]byte(`{"items":[]}`))
			} else {
				_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"svc-1","namespace":"default"},"spec":{"type":"LoadBalancer","ports":[]},"status":{"loadBalancer":{"ingress":[]}}}]}`))
			}
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/namespaces/default/services/svc-1/status":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(kube.Close)

	marmotd = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/server":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/kubernetes-engine/ke-1/loadbalancer/vip":
			requestVipCalled.Store(true)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"vip":"192.168.1.100","fqdn":"svc-1.default.demo.host.labo.local"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/kubernetes-engine/ke-1/loadbalancer/vip/default/svc-1":
			releaseVipCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected marmotd request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(marmotd.Close)

	return kube, marmotd, requestVipCalled, releaseVipCalled
}

func TestReconcileRequestsVipWhenCloudControllerManagerDisabled(t *testing.T) {
	kube, marmotd, requestVipCalled, _ := newReconcileVipTestServers(t, nil)

	client := &kubeClient{httpClient: kube.Client(), server: kube.URL}
	mClient := &marmotdClient{httpClient: marmotd.Client(), baseURL: marmotd.URL + "/api/v1", apiKey: "test-token", keID: "ke-1"}
	haproxyConfigPath := filepath.Join(t.TempDir(), "haproxy.cfg")

	reconcile(context.Background(), client, mClient, haproxyConfigPath, "", "", map[string]string{}, false)

	if !requestVipCalled.Load() {
		t.Fatalf("expected requestVip to be called when cloud-controller-manager is disabled")
	}
}

func TestReconcileSkipsRequestVipWhenCloudControllerManagerEnabled(t *testing.T) {
	kube, marmotd, requestVipCalled, _ := newReconcileVipTestServers(t, nil)

	client := &kubeClient{httpClient: kube.Client(), server: kube.URL}
	mClient := &marmotdClient{httpClient: marmotd.Client(), baseURL: marmotd.URL + "/api/v1", apiKey: "test-token", keID: "ke-1"}
	haproxyConfigPath := filepath.Join(t.TempDir(), "haproxy.cfg")

	reconcile(context.Background(), client, mClient, haproxyConfigPath, "", "", map[string]string{}, true)

	if requestVipCalled.Load() {
		t.Fatalf("expected requestVip NOT to be called when cloud-controller-manager is enabled")
	}
}

func TestReconcileReleasesVipWhenCloudControllerManagerDisabled(t *testing.T) {
	known := map[string]string{"default/svc-1": "192.168.1.100"}
	kube, marmotd, _, releaseVipCalled := newReconcileVipTestServers(t, known)

	client := &kubeClient{httpClient: kube.Client(), server: kube.URL}
	mClient := &marmotdClient{httpClient: marmotd.Client(), baseURL: marmotd.URL + "/api/v1", apiKey: "test-token", keID: "ke-1"}
	haproxyConfigPath := filepath.Join(t.TempDir(), "haproxy.cfg")

	reconcile(context.Background(), client, mClient, haproxyConfigPath, "", "", known, false)

	if !releaseVipCalled.Load() {
		t.Fatalf("expected releaseVip to be called when cloud-controller-manager is disabled")
	}
}

func TestReconcileSkipsReleaseVipWhenCloudControllerManagerEnabled(t *testing.T) {
	known := map[string]string{"default/svc-1": "192.168.1.100"}
	kube, marmotd, _, releaseVipCalled := newReconcileVipTestServers(t, known)

	client := &kubeClient{httpClient: kube.Client(), server: kube.URL}
	mClient := &marmotdClient{httpClient: marmotd.Client(), baseURL: marmotd.URL + "/api/v1", apiKey: "test-token", keID: "ke-1"}
	haproxyConfigPath := filepath.Join(t.TempDir(), "haproxy.cfg")

	reconcile(context.Background(), client, mClient, haproxyConfigPath, "", "", known, true)

	if releaseVipCalled.Load() {
		t.Fatalf("expected releaseVip NOT to be called when cloud-controller-manager is enabled")
	}
}
