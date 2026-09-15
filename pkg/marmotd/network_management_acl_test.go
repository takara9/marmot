package marmotd

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

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

func TestValidateExistingManagementNetwork_AcceptsReservedConfiguration(t *testing.T) {
	vnet := newReservedManagementNetworkForTest()
	ipNetID := "ipnet-1"
	vnet.Spec.IpNetworkId = &ipNetID

	if err := validateExistingManagementNetwork(vnet); err != nil {
		t.Fatalf("validateExistingManagementNetwork() error = %v, want nil", err)
	}
}

func TestValidateExistingManagementNetwork_RejectsIncompatibleConfiguration(t *testing.T) {
	vnet := newReservedManagementNetworkForTest()
	ipNetID := "ipnet-1"
	vnet.Spec.IpNetworkId = &ipNetID
	vnet.Spec.IPNetworkAddress = util.StringPtr("10.99.0.0/24")
	vnet.Spec.BridgeName = util.StringPtr("virbr0")
	overlayMode := api.None
	vnet.Spec.OverlayMode = &overlayMode
	labels := map[string]interface{}{}
	vnet.Metadata.Labels = &labels

	err := validateExistingManagementNetwork(vnet)
	if err == nil {
		t.Fatal("validateExistingManagementNetwork() error = nil, want incompatibility error")
	}
	for _, want := range []string{
		"cidr must be " + ManagementNetworkCIDR,
		"overlayMode must be geneve",
		"bridgeName must be " + api.OVNIntegrationBridgeName,
		"label " + api.NetworkLabelACLEnforced + " must be true",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validateExistingManagementNetwork() error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestReconcileManagementNetworkSpec_NormalizesReservedSettings(t *testing.T) {
	vnet := api.VirtualNetwork{
		Metadata: api.Metadata{
			Name: ManagementNetworkName,
			Labels: &map[string]interface{}{
				"keep": "me",
			},
		},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("192.168.0.0/24"),
			BridgeName:       util.StringPtr("virbr0"),
		},
	}

	reconcileManagementNetworkSpec(&vnet)

	if vnet.Spec.IPNetworkAddress == nil || *vnet.Spec.IPNetworkAddress != ManagementNetworkCIDR {
		t.Fatalf("IPNetworkAddress = %v, want %q", vnet.Spec.IPNetworkAddress, ManagementNetworkCIDR)
	}
	if vnet.Spec.BridgeName == nil || *vnet.Spec.BridgeName != api.OVNIntegrationBridgeName {
		t.Fatalf("BridgeName = %v, want %q", vnet.Spec.BridgeName, api.OVNIntegrationBridgeName)
	}
	if vnet.Spec.OverlayMode == nil || *vnet.Spec.OverlayMode != api.Geneve {
		t.Fatalf("OverlayMode = %v, want %q", vnet.Spec.OverlayMode, api.Geneve)
	}
	if vnet.Metadata.Labels == nil || (*vnet.Metadata.Labels)["keep"] != "me" {
		t.Fatalf("Labels = %#v, want preserved custom label", vnet.Metadata.Labels)
	}
	if got, ok := (*vnet.Metadata.Labels)[api.NetworkLabelACLEnforced].(string); !ok || got != "true" {
		t.Fatalf("ACL label = %#v, want %q", (*vnet.Metadata.Labels)[api.NetworkLabelACLEnforced], "true")
	}
}

func newReservedManagementNetworkForTest() api.VirtualNetwork {
	labels := map[string]interface{}{
		api.NetworkLabelACLEnforced: "true",
	}
	overlayMode := api.Geneve
	return api.VirtualNetwork{
		Metadata: api.Metadata{
			Name:   ManagementNetworkName,
			Labels: &labels,
		},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr(ManagementNetworkCIDR),
			BridgeName:       util.StringPtr(api.OVNIntegrationBridgeName),
			OverlayMode:      &overlayMode,
		},
	}
}
