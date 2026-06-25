package controller

import (
	"errors"
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
		Spec: api.ApplicationLoadBalancerSpec{
			BindPublicIpAddress:    "192.168.1.120",
			InternalVirtualNetwork: "web-servers",
			Listeners: []api.ApplicationLoadBalancerListener{{
				Name:                   "web-http",
				Protocol:               "HTTP",
				VipPort:                80,
				BackendPort:            8080,
				LoadBalancingAlgorithm: "roundrobin",
				BackendSelector: api.ApplicationLoadBalancerLabelSelector{
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
	if !strings.Contains(cfg, "option forwardfor if-none") {
		t.Fatalf("generated cfg does not enable X-Forwarded-For propagation: %s", cfg)
	}
	if !strings.Contains(cfg, "http-request set-header X-Real-IP %[src]") {
		t.Fatalf("generated cfg does not set X-Real-IP header: %s", cfg)
	}
	if !strings.Contains(cfg, "server srv-1-web-a 172.16.10.11:8080 check") {
		t.Fatalf("generated cfg does not contain backend web-a entry: %s", cfg)
	}
	if !strings.Contains(cfg, "server srv-2-web-b 172.16.10.12:8080 check") {
		t.Fatalf("generated cfg does not contain backend web-b entry: %s", cfg)
	}
}

func TestBuildApplicationLoadBalancerHAProxyConfig_TCPListenerDoesNotInjectHTTPHeaders(t *testing.T) {
	loadBalancer := api.ApplicationLoadBalancer{
		ApiVersion: "v1",
		Kind:       "ApplicationLoadBalancer",
		Metadata: api.Metadata{
			Name: "lb-tcp",
			Id:   "lb-tcp-1",
		},
		Spec: api.ApplicationLoadBalancerSpec{
			BindPublicIpAddress:    "192.168.1.130",
			InternalVirtualNetwork: "db-net",
			Listeners: []api.ApplicationLoadBalancerListener{{
				Name:                   "db-tcp",
				Protocol:               "TCP",
				VipPort:                3306,
				BackendPort:            3306,
				LoadBalancingAlgorithm: "roundrobin",
				BackendSelector: api.ApplicationLoadBalancerLabelSelector{
					MatchLabels: map[string]string{"app": "db"},
				},
			}},
		},
	}

	cfg, err := buildApplicationLoadBalancerHAProxyConfig(loadBalancer, map[string][]applicationLoadBalancerBackendServer{
		"db-tcp": {{Name: "db-a", IP: "172.16.20.11"}},
	})
	if err != nil {
		t.Fatalf("buildApplicationLoadBalancerHAProxyConfig() failed: %v", err)
	}
	if strings.Contains(cfg, "option forwardfor if-none") {
		t.Fatalf("generated cfg must not include HTTP client IP propagation for TCP listeners: %s", cfg)
	}
	if strings.Contains(cfg, "http-request set-header X-Real-IP %[src]") {
		t.Fatalf("generated cfg must not include X-Real-IP header injection for TCP listeners: %s", cfg)
	}
}

func TestBuildApplicationLoadBalancerHAProxyConfig_NormalizesCIDRBindAddress(t *testing.T) {
	loadBalancer := api.ApplicationLoadBalancer{
		ApiVersion: "v1",
		Kind:       "ApplicationLoadBalancer",
		Metadata: api.Metadata{
			Name: "lb-cidr",
			Id:   "lb-cidr-1",
		},
		Spec: api.ApplicationLoadBalancerSpec{
			BindPublicIpAddress:    "192.168.1.130/24",
			InternalVirtualNetwork: "db-net",
			Listeners: []api.ApplicationLoadBalancerListener{{
				Name:                   "db-tcp",
				Protocol:               "TCP",
				VipPort:                3306,
				BackendPort:            3306,
				LoadBalancingAlgorithm: "roundrobin",
				BackendSelector: api.ApplicationLoadBalancerLabelSelector{
					MatchLabels: map[string]string{"app": "db"},
				},
			}},
		},
	}

	cfg, err := buildApplicationLoadBalancerHAProxyConfig(loadBalancer, map[string][]applicationLoadBalancerBackendServer{
		"db-tcp": {{Name: "db-a", IP: "172.16.20.11"}},
	})
	if err != nil {
		t.Fatalf("buildApplicationLoadBalancerHAProxyConfig() failed: %v", err)
	}
	if !strings.Contains(cfg, "bind 192.168.1.130:3306") {
		t.Fatalf("generated cfg does not normalize CIDR bind address: %s", cfg)
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
		Spec: api.ApplicationLoadBalancerSpec{
			BindPublicIpAddress:    "192.168.1.120",
			InternalVirtualNetwork: "web-servers",
			Listeners: []api.ApplicationLoadBalancerListener{{
				Name:                   "web-http",
				Protocol:               "HTTP",
				VipPort:                80,
				BackendPort:            8080,
				LoadBalancingAlgorithm: "roundrobin",
				BackendSelector: api.ApplicationLoadBalancerLabelSelector{
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

func TestIsRetryableApplicationLoadBalancerPendingError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "internalVirtualNetwork not found is retryable",
			msg:  `internalVirtualNetwork "app-net" is not found`,
			want: true,
		},
		{
			name: "uppercase variant is retryable",
			msg:  `internalVirtualNetwork "WebServers" is not found`,
			want: true,
		},
		{
			name: "unrelated error is not retryable",
			msg:  "spec.internalVirtualNetwork is required",
			want: false,
		},
		{
			name: "reserved network error is not retryable",
			msg:  `spec.internalVirtualNetwork "default" is reserved`,
			want: false,
		},
		{
			name: "nil error is not retryable",
			msg:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.msg != "" {
				err = errors.New(tt.msg)
			}
			got := isRetryableApplicationLoadBalancerPendingError(err)
			if got != tt.want {
				t.Fatalf("isRetryableApplicationLoadBalancerPendingError() = %v, want %v, msg=%q", got, tt.want, tt.msg)
			}
		})
	}
}
