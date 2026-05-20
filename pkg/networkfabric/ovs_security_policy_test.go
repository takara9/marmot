package networkfabric

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestBuildSecurityPolicyFlowsWithNilPolicy(t *testing.T) {
	flows, err := buildSecurityPolicyFlows(nil)
	if err != nil {
		t.Fatalf("buildSecurityPolicyFlows(nil) returned error: %v", err)
	}
	if len(flows) != 0 {
		t.Fatalf("expected no flows for nil policy, got %d", len(flows))
	}
}

func TestBuildSecurityPolicyFlowsWithValidRules(t *testing.T) {
	policy := &api.VirtualNetworkSecurityPolicy{
		DefaultAction: api.Deny,
		Rules: []api.VirtualNetworkSecurityRule{
			{
				Direction:    api.Ingress,
				Protocol:     api.Tcp,
				RemoteCidr:   "10.0.0.0/24",
				PortRangeMin: 22,
				PortRangeMax: 22,
			},
			{
				Direction:    api.Egress,
				Protocol:     api.Udp,
				RemoteCidr:   "0.0.0.0/0",
				PortRangeMin: 53,
				PortRangeMax: 53,
			},
		},
	}

	flows, err := buildSecurityPolicyFlows(policy)
	if err != nil {
		t.Fatalf("buildSecurityPolicyFlows(valid) returned error: %v", err)
	}
	if len(flows) < 7 {
		t.Fatalf("expected at least baseline flows + rules, got %d", len(flows))
	}

	joined := strings.Join(flows, "\n")
	if !strings.Contains(joined, "ct_state=+trk+est") {
		t.Fatalf("expected established conntrack allow flow, got:\n%s", joined)
	}
	if !strings.Contains(joined, "nw_src=10.0.0.0/24") {
		t.Fatalf("expected ingress source CIDR match, got:\n%s", joined)
	}
	if !strings.Contains(joined, "nw_dst=0.0.0.0/0") {
		t.Fatalf("expected egress destination CIDR match, got:\n%s", joined)
	}
	if !strings.Contains(joined, "actions=ct(commit),NORMAL") {
		t.Fatalf("expected commit action in allow rules, got:\n%s", joined)
	}
	if !strings.Contains(joined, "ct_state=+trk+new,actions=drop") {
		t.Fatalf("expected default drop for unmatched new tracked packets, got:\n%s", joined)
	}
}

func TestBuildSecurityPolicyFlowsWithDefaultAllow(t *testing.T) {
	policy := &api.VirtualNetworkSecurityPolicy{
		DefaultAction: api.Allow,
		Rules: []api.VirtualNetworkSecurityRule{
			{
				Direction:    api.Ingress,
				Protocol:     api.Tcp,
				RemoteCidr:   "10.0.0.0/24",
				PortRangeMin: 443,
				PortRangeMax: 443,
			},
		},
	}

	flows, err := buildSecurityPolicyFlows(policy)
	if err != nil {
		t.Fatalf("buildSecurityPolicyFlows(default allow) returned error: %v", err)
	}

	joined := strings.Join(flows, "\n")
	if !strings.Contains(joined, "ct_state=+trk+new,actions=ct(commit),NORMAL") {
		t.Fatalf("expected tracked new packets to be allowed for defaultAction=allow, got:\n%s", joined)
	}
}

func TestBuildSecurityPolicyFlowsRejectsInvalidDefaultAction(t *testing.T) {
	policy := &api.VirtualNetworkSecurityPolicy{
		DefaultAction: api.VirtualNetworkSecurityPolicyDefaultAction("block"),
		Rules: []api.VirtualNetworkSecurityRule{
			{
				Direction:    api.Ingress,
				Protocol:     api.Tcp,
				RemoteCidr:   "10.0.0.0/24",
				PortRangeMin: 22,
				PortRangeMax: 22,
			},
		},
	}

	_, err := buildSecurityPolicyFlows(policy)
	if err == nil {
		t.Fatal("expected error for invalid defaultAction")
	}
}

func TestBuildSecurityPolicyFlowsRejectsInvalidRule(t *testing.T) {
	policy := &api.VirtualNetworkSecurityPolicy{
		DefaultAction: api.Deny,
		Rules: []api.VirtualNetworkSecurityRule{
			{
				Direction:    api.Ingress,
				Protocol:     api.Tcp,
				RemoteCidr:   "10.0.0.999/24",
				PortRangeMin: 22,
				PortRangeMax: 22,
			},
		},
	}

	_, err := buildSecurityPolicyFlows(policy)
	if err == nil {
		t.Fatal("expected error for invalid remote CIDR")
	}
}
