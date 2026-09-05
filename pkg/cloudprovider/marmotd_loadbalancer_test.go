package cloudprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/takara9/marmot/pkg/client"
)

func newTestMarmotdLoadBalancerEndpoint(t *testing.T, handler http.HandlerFunc) *client.MarmotEndpoint {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	endpoint, err := client.NewMarmotdEp("http", ts.URL[len("http://"):], "/api/v1", 5, false)
	if err != nil {
		t.Fatalf("NewMarmotdEp() failed: %v", err)
	}
	return endpoint
}

func TestMarmotdLoadBalancerEnsureLoadBalancer(t *testing.T) {
	var gotMethod, gotPath string
	endpoint := newTestMarmotdLoadBalancerEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"vip":"192.168.1.100","fqdn":"svc-1.default.demo.host.labo.local"}`))
	})

	lb := NewMarmotdLoadBalancer(endpoint)
	vip, err := lb.EnsureLoadBalancer(context.Background(), "ke-1", LoadBalancerService{Namespace: "default", Name: "svc-1"})
	if err != nil {
		t.Fatalf("EnsureLoadBalancer() failed: %v", err)
	}
	if vip != "192.168.1.100" {
		t.Fatalf("vip = %q, want %q", vip, "192.168.1.100")
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/kubernetes-engine/ke-1/loadbalancer/vip" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v1/kubernetes-engine/ke-1/loadbalancer/vip")
	}
}

func TestMarmotdLoadBalancerEnsureLoadBalancerError(t *testing.T) {
	endpoint := newTestMarmotdLoadBalancerEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":1,"message":"boom"}`))
	})

	lb := NewMarmotdLoadBalancer(endpoint)
	if _, err := lb.EnsureLoadBalancer(context.Background(), "ke-1", LoadBalancerService{Namespace: "default", Name: "svc-1"}); err == nil {
		t.Fatalf("expected error from EnsureLoadBalancer()")
	}
}

func TestMarmotdLoadBalancerEnsureLoadBalancerDeleted(t *testing.T) {
	var gotMethod, gotPath string
	endpoint := newTestMarmotdLoadBalancerEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ke-1","message":"VIP released"}`))
	})

	lb := NewMarmotdLoadBalancer(endpoint)
	if err := lb.EnsureLoadBalancerDeleted(context.Background(), "ke-1", LoadBalancerService{Namespace: "default", Name: "svc-1"}); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted() failed: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/kubernetes-engine/ke-1/loadbalancer/vip/default/svc-1" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v1/kubernetes-engine/ke-1/loadbalancer/vip/default/svc-1")
	}
}
