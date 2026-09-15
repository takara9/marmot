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
	if len(rules) != 4 {
		t.Fatalf("len(rules) = %d, want 4 (2 allow + 2 deny)", len(rules))
	}

	want0 := "ip4 && ip4.dst==10.245.0.1/32 && tcp.dst==9090"
	if rules[0].Match != want0 || rules[0].Action != "allow-related" || rules[0].Priority != ManagementNetworkACLAllowPriority {
		t.Fatalf("rules[0] = %+v, want match=%q action=allow-related priority=%d", rules[0], want0, ManagementNetworkACLAllowPriority)
	}

	want1 := "ip4 && ip4.dst==10.245.0.1/32 && udp.dst==53"
	if rules[1].Match != want1 || rules[1].Action != "allow-related" {
		t.Fatalf("rules[1] = %+v, want match=%q action=allow-related", rules[1], want1)
	}

	ip4Deny := rules[2]
	if ip4Deny.Match != "ip4" || ip4Deny.Action != "drop" || ip4Deny.Priority != ManagementNetworkACLDenyPriority {
		t.Fatalf("rules[2] = %+v, want default ip4 deny rule", ip4Deny)
	}

	ip6Deny := rules[3]
	if ip6Deny.Match != "ip6" || ip6Deny.Action != "drop" || ip6Deny.Priority != ManagementNetworkACLDenyPriority {
		t.Fatalf("rules[3] = %+v, want default ip6 deny rule", ip6Deny)
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
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2 (only default ip4/ip6 deny, all allow entries invalid)", len(rules))
	}
	if rules[0].Action != "drop" || rules[1].Action != "drop" {
		t.Fatalf("rules = %+v, want drop", rules)
	}
}

func TestBuildManagementNetworkACLRules_NilConfigReturnsOnlyDeny(t *testing.T) {
	rules := BuildManagementNetworkACLRules(nil)
	if len(rules) != 2 || rules[0].Action != "drop" || rules[1].Action != "drop" {
		t.Fatalf("rules = %+v, want ip4 and ip6 default deny rules", rules)
	}
	if rules[0].Match != "ip4" || rules[1].Match != "ip6" {
		t.Fatalf("rules = %+v, want ip4 then ip6 deny", rules)
	}
}
