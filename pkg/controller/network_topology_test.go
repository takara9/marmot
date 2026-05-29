package controller

import (
	"reflect"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestPickDeterministicHubNode(t *testing.T) {
	nodes := map[string]struct{}{
		"marmot3": {},
		"marmot1": {},
		"marmot2": {},
	}

	hub := pickDeterministicHubNode(nodes)
	if hub != "marmot1" {
		t.Fatalf("unexpected hub node: got %q, want %q", hub, "marmot1")
	}
}

func TestBuildVxlanHubSpokePeerIPs_SpokeNode(t *testing.T) {
	participantNodes := map[string]struct{}{
		"marmot1": {},
		"marmot2": {},
		"marmot3": {},
	}
	ipByNode := map[string]string{
		"marmot1": "10.0.0.1",
		"marmot2": "10.0.0.2",
		"marmot3": "10.0.0.3",
	}

	peers := buildVxlanHubSpokePeerIPs("marmot2", "marmot1", participantNodes, ipByNode)
	want := []string{"10.0.0.1"}
	if !reflect.DeepEqual(peers, want) {
		t.Fatalf("unexpected peers: got %v, want %v", peers, want)
	}
}

func TestBuildVxlanHubSpokePeerIPs_HubNode(t *testing.T) {
	participantNodes := map[string]struct{}{
		"marmot1": {},
		"marmot2": {},
		"marmot3": {},
	}
	ipByNode := map[string]string{
		"marmot1": "10.0.0.1",
		"marmot2": "10.0.0.2",
		"marmot3": "10.0.0.3",
	}

	peers := buildVxlanHubSpokePeerIPs("marmot1", "marmot1", participantNodes, ipByNode)
	if len(peers) != 2 {
		t.Fatalf("unexpected peer count: got %d, want 2", len(peers))
	}

	gotSet := map[string]struct{}{}
	for _, ip := range peers {
		gotSet[ip] = struct{}{}
	}
	if _, ok := gotSet["10.0.0.2"]; !ok {
		t.Fatalf("missing spoke peer ip: %s", "10.0.0.2")
	}
	if _, ok := gotSet["10.0.0.3"]; !ok {
		t.Fatalf("missing spoke peer ip: %s", "10.0.0.3")
	}
}

func TestBuildVxlanHubSpokePeerIPs_MissingHubIP(t *testing.T) {
	participantNodes := map[string]struct{}{
		"marmot1": {},
		"marmot2": {},
	}
	ipByNode := map[string]string{
		"marmot2": "10.0.0.2",
	}

	peers := buildVxlanHubSpokePeerIPs("marmot2", "marmot1", participantNodes, ipByNode)
	if len(peers) != 0 {
		t.Fatalf("unexpected peers: got %v, want empty", peers)
	}
}

func TestIsGeneveOverlay(t *testing.T) {
	mode := api.Geneve
	vnet := api.VirtualNetwork{Spec: api.VirtualNetworkSpec{OverlayMode: &mode}}
	if !isGeneveOverlay(vnet) {
		t.Fatalf("expected geneve overlay to be detected")
	}
}

func TestIsGeneveOverlay_NonGeneve(t *testing.T) {
	vxlan := api.Vxlan
	vnet := api.VirtualNetwork{Spec: api.VirtualNetworkSpec{OverlayMode: &vxlan}}
	if isGeneveOverlay(vnet) {
		t.Fatalf("vxlan overlay must not be treated as geneve")
	}

	vnet.Spec.OverlayMode = nil
	if isGeneveOverlay(vnet) {
		t.Fatalf("nil overlay must not be treated as geneve")
	}
}
