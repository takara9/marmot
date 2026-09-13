package marmotd

import "testing"

func TestBuildManagementNetworkACLRules_ValidAllowEntriesPlusDefaultDeny(t *testing.T) {
	cfg := &MarmotdConfig{
		ManagementNetworkACLAllow: []ManagementNetworkACLAllowEntry{
			{Description: "prometheus", CIDR: "10.245.0.1/32", Protocol: "TCP", Port: 9090},
			{Description: "dns", CIDR: "10.245.0.1", Protocol: "udp", Port: 53},
		},
	}

	rules := BuildManagementNetworkACLRules(cfg)
	if len(rules) != 3 {
		t.Fatalf("len(rules) = %d, want 3 (2 allow + 1 deny)", len(rules))
	}

	want0 := "ip4 && ip4.dst==10.245.0.1/32 && tcp.dst==9090"
	if rules[0].Match != want0 || rules[0].Action != "allow-related" || rules[0].Priority != ManagementNetworkACLAllowPriority {
		t.Fatalf("rules[0] = %+v, want match=%q action=allow-related priority=%d", rules[0], want0, ManagementNetworkACLAllowPriority)
	}

	want1 := "ip4 && ip4.dst==10.245.0.1/32 && udp.dst==53"
	if rules[1].Match != want1 || rules[1].Action != "allow-related" {
		t.Fatalf("rules[1] = %+v, want match=%q action=allow-related", rules[1], want1)
	}

	last := rules[len(rules)-1]
	if last.Match != "ip4" || last.Action != "drop" || last.Priority != ManagementNetworkACLDenyPriority {
		t.Fatalf("last rule = %+v, want default deny rule", last)
	}
}

func TestBuildManagementNetworkACLRules_SkipsInvalidEntries(t *testing.T) {
	cfg := &MarmotdConfig{
		ManagementNetworkACLAllow: []ManagementNetworkACLAllowEntry{
			{Description: "invalid-cidr", CIDR: "not-an-ip", Protocol: "tcp", Port: 9090},
			{Description: "invalid-protocol", CIDR: "10.245.0.1/32", Protocol: "icmp", Port: 0},
			{Description: "invalid-port", CIDR: "10.245.0.1/32", Protocol: "tcp", Port: 70000},
		},
	}

	rules := BuildManagementNetworkACLRules(cfg)
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1 (only default deny, all allow entries invalid)", len(rules))
	}
	if rules[0].Action != "drop" {
		t.Fatalf("rules[0].Action = %q, want drop", rules[0].Action)
	}
}

func TestBuildManagementNetworkACLRules_NilConfigReturnsOnlyDeny(t *testing.T) {
	rules := BuildManagementNetworkACLRules(nil)
	if len(rules) != 1 || rules[0].Action != "drop" {
		t.Fatalf("rules = %+v, want single default deny rule", rules)
	}
}
