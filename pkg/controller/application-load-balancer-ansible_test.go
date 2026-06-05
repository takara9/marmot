package controller

import (
	"strings"
	"testing"

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

	hashA := desiredApplicationLoadBalancerConfigHash(loadBalancer, map[string][]applicationLoadBalancerBackendServer{
		"web-http": {{Name: "web-a", IP: "172.16.10.11"}},
	})
	hashB := desiredApplicationLoadBalancerConfigHash(loadBalancer, map[string][]applicationLoadBalancerBackendServer{
		"web-http": {{Name: "web-b", IP: "172.16.10.12"}},
	})

	if hashA == hashB {
		t.Fatalf("hash must differ when backend set changes")
	}
}