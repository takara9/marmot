package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/takara9/marmot/pkg/cloudprovider"
)

// fakeInstances は pkg/cloudprovider.Instances のテスト用フェイク実装。
type fakeInstances struct {
	metadata map[string]*cloudprovider.InstanceMetadata
	err      map[string]error
}

var _ cloudprovider.Instances = (*fakeInstances)(nil)

func (f *fakeInstances) InstanceExists(ctx context.Context, nodeName string) (bool, error) {
	return true, nil
}

func (f *fakeInstances) InstanceShutdown(ctx context.Context, nodeName string) (bool, error) {
	return false, nil
}

func (f *fakeInstances) InstanceMetadata(ctx context.Context, nodeName string) (*cloudprovider.InstanceMetadata, error) {
	if err, ok := f.err[nodeName]; ok {
		return nil, err
	}
	return f.metadata[nodeName], nil
}

func TestReconcileSetsAddressesProviderIDAndRemovesUninitializedTaint(t *testing.T) {
	var patchedStatusPaths, patchedSpecPaths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[{
				"metadata":{"name":"node-1"},
				"spec":{"providerID":"","taints":[
					{"key":"node.cloudprovider.kubernetes.io/uninitialized","effect":"NoSchedule"},
					{"key":"node.kubernetes.io/not-ready","effect":"NoSchedule"}
				]},
				"status":{"addresses":[]}
			}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/nodes/node-1/status":
			patchedStatusPaths = append(patchedStatusPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/nodes/node-1":
			patchedSpecPaths = append(patchedSpecPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	kc := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	instances := &fakeInstances{
		metadata: map[string]*cloudprovider.InstanceMetadata{
			"node-1": {
				ProviderID: "marmot://ke-1/srv-1",
				NodeAddresses: []cloudprovider.NodeAddress{
					{Type: cloudprovider.NodeHostName, Address: "node-1"},
					{Type: cloudprovider.NodeInternalIP, Address: "172.16.1.10"},
					{Type: cloudprovider.NodeExternalIP, Address: "192.168.1.50"},
				},
			},
		},
	}

	reconcile(context.Background(), kc, instances)

	if len(patchedStatusPaths) != 1 {
		t.Fatalf("expected 1 status patch, got %d", len(patchedStatusPaths))
	}
	if len(patchedSpecPaths) != 1 {
		t.Fatalf("expected 1 spec patch, got %d", len(patchedSpecPaths))
	}
}

func TestReconcileSkipsSpecPatchWhenAlreadyUpToDate(t *testing.T) {
	var patchedSpecPaths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[{
				"metadata":{"name":"node-1"},
				"spec":{"providerID":"marmot://ke-1/srv-1","taints":[]},
				"status":{"addresses":[{"type":"InternalIP","address":"172.16.1.10"},{"type":"ExternalIP","address":"192.168.1.50"}]}
			}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/nodes/node-1":
			patchedSpecPaths = append(patchedSpecPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/nodes/node-1/status":
			t.Fatalf("addresses already up to date, status should not be patched")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	kc := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	instances := &fakeInstances{
		metadata: map[string]*cloudprovider.InstanceMetadata{
			"node-1": {
				ProviderID: "marmot://ke-1/srv-1",
				NodeAddresses: []cloudprovider.NodeAddress{
					{Type: cloudprovider.NodeInternalIP, Address: "172.16.1.10"},
					{Type: cloudprovider.NodeExternalIP, Address: "192.168.1.50"},
				},
			},
		},
	}

	reconcile(context.Background(), kc, instances)

	if len(patchedSpecPaths) != 0 {
		t.Fatalf("expected no spec patch when providerID/taints already up to date, got %d", len(patchedSpecPaths))
	}
}

func TestReconcileContinuesOnInstanceMetadataError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes" {
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"node-1"},"spec":{},"status":{"addresses":[]}}]}`))
			return
		}
		t.Fatalf("unexpected request when metadata fetch fails: %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()

	kc := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	instances := &fakeInstances{
		err: map[string]error{"node-1": errBoom},
	}

	// Should not panic and should not attempt any patch call.
	reconcile(context.Background(), kc, instances)
}

func TestRemoveTaintFiltersOnlyMatchingKey(t *testing.T) {
	taints := []nodeTaint{
		{Key: "node.cloudprovider.kubernetes.io/uninitialized", Effect: "NoSchedule"},
		{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"},
	}
	remaining, removed := removeTaint(taints, kubernetesEngineNodeUninitializedTaintKey)
	if !removed {
		t.Fatalf("expected removed=true")
	}
	if len(remaining) != 1 || remaining[0].Key != "node.kubernetes.io/not-ready" {
		t.Fatalf("unexpected remaining taints: %+v", remaining)
	}
}

func TestRemoveTaintNoOpWhenKeyAbsent(t *testing.T) {
	taints := []nodeTaint{{Key: "node.kubernetes.io/not-ready", Effect: "NoSchedule"}}
	remaining, removed := removeTaint(taints, kubernetesEngineNodeUninitializedTaintKey)
	if removed {
		t.Fatalf("expected removed=false")
	}
	if len(remaining) != 1 {
		t.Fatalf("unexpected remaining taints: %+v", remaining)
	}
}

type staticError string

func (e staticError) Error() string { return string(e) }

const errBoom = staticError("boom")

// fakeLoadBalancer は pkg/cloudprovider.LoadBalancer のテスト用フェイク実装。
type fakeLoadBalancer struct {
	ensureCalls []string
	deleteCalls []string
	ensureVIP   string
	ensureErr   error
	deleteErr   error
}

var _ cloudprovider.LoadBalancer = (*fakeLoadBalancer)(nil)

func (f *fakeLoadBalancer) EnsureLoadBalancer(ctx context.Context, clusterID string, svc cloudprovider.LoadBalancerService) (string, error) {
	f.ensureCalls = append(f.ensureCalls, svc.Namespace+"/"+svc.Name)
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	return f.ensureVIP, nil
}

func (f *fakeLoadBalancer) EnsureLoadBalancerDeleted(ctx context.Context, clusterID string, svc cloudprovider.LoadBalancerService) error {
	f.deleteCalls = append(f.deleteCalls, svc.Namespace+"/"+svc.Name)
	return f.deleteErr
}

func TestReconcileLoadBalancerAllocatesVipForNewService(t *testing.T) {
	var patchedStatusPaths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			_, _ = w.Write([]byte(`{"items":[{
				"metadata":{"name":"svc-1","namespace":"default"},
				"spec":{"type":"LoadBalancer"},
				"status":{"loadBalancer":{"ingress":[]}}
			}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/namespaces/default/services/svc-1/status":
			patchedStatusPaths = append(patchedStatusPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	kc := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	lb := &fakeLoadBalancer{ensureVIP: "192.168.1.100"}
	known := make(map[string]struct{})

	reconcileLoadBalancer(context.Background(), kc, lb, "ke-1", known)

	if len(lb.ensureCalls) != 1 || lb.ensureCalls[0] != "default/svc-1" {
		t.Fatalf("unexpected EnsureLoadBalancer calls: %+v", lb.ensureCalls)
	}
	if len(patchedStatusPaths) != 1 {
		t.Fatalf("expected 1 status patch, got %d", len(patchedStatusPaths))
	}
	if _, ok := known["default/svc-1"]; !ok {
		t.Fatalf("expected known VIPs to contain default/svc-1, got %+v", known)
	}
}

func TestReconcileLoadBalancerSkipsServiceWithExistingVip(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/services" {
			_, _ = w.Write([]byte(`{"items":[{
				"metadata":{"name":"svc-1","namespace":"default"},
				"spec":{"type":"LoadBalancer"},
				"status":{"loadBalancer":{"ingress":[{"ip":"192.168.1.100"}]}}
			}]}`))
			return
		}
		t.Fatalf("unexpected request when VIP already set: %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()

	kc := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	lb := &fakeLoadBalancer{}
	known := make(map[string]struct{})

	reconcileLoadBalancer(context.Background(), kc, lb, "ke-1", known)

	if len(lb.ensureCalls) != 0 {
		t.Fatalf("expected no EnsureLoadBalancer calls, got %+v", lb.ensureCalls)
	}
	if _, ok := known["default/svc-1"]; !ok {
		t.Fatalf("expected known VIPs to contain default/svc-1, got %+v", known)
	}
}

func TestReconcileLoadBalancerReleasesVipForRemovedService(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/services" {
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()

	kc := &kubeClient{httpClient: ts.Client(), server: ts.URL}
	lb := &fakeLoadBalancer{}
	known := map[string]struct{}{"default/svc-1": {}}

	reconcileLoadBalancer(context.Background(), kc, lb, "ke-1", known)

	if len(lb.deleteCalls) != 1 || lb.deleteCalls[0] != "default/svc-1" {
		t.Fatalf("unexpected EnsureLoadBalancerDeleted calls: %+v", lb.deleteCalls)
	}
	if _, ok := known["default/svc-1"]; ok {
		t.Fatalf("expected default/svc-1 to be removed from known VIPs, got %+v", known)
	}
}
