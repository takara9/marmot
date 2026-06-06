package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
)

func TestBuildApplicationLoadBalancerHAProxyConfig_WithResolvedBackends(t *testing.T) {
	loadBalancer := api.ApplicationLoadBalancer{
		ApiVersion: "v1",
		Kind:       "ApplicationLoadBalancer",
		Metadata: api.Metadata{
			Name: "lb-web",
			Id:   "lb123",
		},
		Spec: api.LoadBalancerSpec{
			BindPublicIpAddress:    "192.168.1.120",
			InternalVirtualNetwork: "web-servers",
			Listeners: []api.LoadBalancerListener{{
				Name:                   "web-http",
				Protocol:               "HTTP",
				VipPort:                80,
				BackendPort:            8080,
				LoadBalancingAlgorithm: "roundrobin",
				BackendSelector: api.LoadBalancerLabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
			}},
		},
	}

	backends := map[string][]applicationLoadBalancerBackendServer{
		"web-http": {
			{Name: "web-a", IP: "172.16.10.11"},
			{Name: "web-b", IP: "172.16.10.12"},
		},
	}

	cfg, err := buildApplicationLoadBalancerHAProxyConfig(loadBalancer, backends)
	if err != nil {
		t.Fatalf("buildApplicationLoadBalancerHAProxyConfig() failed: %v", err)
	}
	if !strings.Contains(cfg, "server srv-1-web-a 172.16.10.11:8080 check") {
		t.Fatalf("generated cfg does not contain backend web-a entry: %s", cfg)
	}
	if !strings.Contains(cfg, "server srv-2-web-b 172.16.10.12:8080 check") {
		t.Fatalf("generated cfg does not contain backend web-b entry: %s", cfg)
	}
}

func TestDesiredApplicationLoadBalancerConfigHash_ChangesWhenBackendsChange(t *testing.T) {
	loadBalancer := api.ApplicationLoadBalancer{
		ApiVersion: "v1",
		Kind:       "ApplicationLoadBalancer",
		Metadata: api.Metadata{
			Name: "lb-web",
			Id:   "lb123",
		},
		Spec: api.LoadBalancerSpec{
			BindPublicIpAddress:    "192.168.1.120",
			InternalVirtualNetwork: "web-servers",
			Listeners: []api.LoadBalancerListener{{
				Name:                   "web-http",
				Protocol:               "HTTP",
				VipPort:                80,
				BackendPort:            8080,
				LoadBalancingAlgorithm: "roundrobin",
				BackendSelector: api.LoadBalancerLabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
			}},
		},
	}

	hashA, err := desiredApplicationLoadBalancerConfigHash(loadBalancer, map[string][]applicationLoadBalancerBackendServer{
		"web-http": {{Name: "web-a", IP: "172.16.10.11"}},
	})
	if err != nil {
		t.Fatalf("desiredApplicationLoadBalancerConfigHash() failed: %v", err)
	}
	hashB, err := desiredApplicationLoadBalancerConfigHash(loadBalancer, map[string][]applicationLoadBalancerBackendServer{
		"web-http": {{Name: "web-b", IP: "172.16.10.12"}},
	})
	if err != nil {
		t.Fatalf("desiredApplicationLoadBalancerConfigHash() failed: %v", err)
	}

	if hashA == hashB {
		t.Fatalf("hash must differ when backend set changes")
	}
}

func TestDecodeApplicationLoadBalancerAgentState_ValidJSON(t *testing.T) {
	state, err := decodeApplicationLoadBalancerAgentState([]byte(`{"lastAppliedHash":"abc","lastAppliedAt":"2026-06-06T00:00:00Z"}`))
	if err != nil {
		t.Fatalf("decodeApplicationLoadBalancerAgentState() failed: %v", err)
	}
	if state.LastAppliedHash != "abc" {
		t.Fatalf("LastAppliedHash = %q, want %q", state.LastAppliedHash, "abc")
	}
	if !state.LastAppliedAt.Equal(time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("LastAppliedAt = %v, want 2026-06-06T00:00:00Z", state.LastAppliedAt)
	}
}

func TestDecodeApplicationLoadBalancerAgentState_RejectsWarningPayload(t *testing.T) {
	_, err := decodeApplicationLoadBalancerAgentState([]byte("Warning: Permanently added '10.0.0.10' (ED25519) to the list of known hosts."))
	if err == nil {
		t.Fatalf("decodeApplicationLoadBalancerAgentState() expected error for non-JSON payload")
	}
}
