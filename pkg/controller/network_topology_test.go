package controller

import (
	"reflect"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
)

func TestIsGeneveOverlay(t *testing.T) {
	mode := api.Geneve
	vnet := api.VirtualNetwork{Spec: api.VirtualNetworkSpec{OverlayMode: &mode}}
	if !isGeneveOverlay(vnet) {
		t.Fatalf("expected geneve overlay to be detected")
	}
}

func TestIsGeneveOverlay_NonGeneve(t *testing.T) {
	none := api.None
	vnet := api.VirtualNetwork{Spec: api.VirtualNetworkSpec{OverlayMode: &none}}
	if isGeneveOverlay(vnet) {
		t.Fatalf("none overlay must not be treated as geneve")
	}

	vnet.Spec.OverlayMode = nil
	if isGeneveOverlay(vnet) {
		t.Fatalf("nil overlay must not be treated as geneve")
	}
}

func TestCollectClusterMemberNodes_DedupAndSort(t *testing.T) {
	now := time.Now()
	marmot1 := "marmot1"
	marmot2 := "marmot2"
	statuses := []api.HostStatus{
		{NodeName: &marmot2, LastUpdated: &now},
		{NodeName: &marmot1, LastUpdated: &now},
		{NodeName: &marmot2, LastUpdated: &now},
		{},
	}

	got := collectClusterMemberNodes(statuses)
	want := []string{"marmot1", "marmot2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected members: got=%v want=%v", got, want)
	}
}

func TestClusterMemberSignature_OrderIndependent(t *testing.T) {
	now := time.Now()
	marmot1 := "marmot1"
	marmot2 := "marmot2"
	statusesA := []api.HostStatus{{NodeName: &marmot1, LastUpdated: &now}, {NodeName: &marmot2, LastUpdated: &now}}
	statusesB := []api.HostStatus{{NodeName: &marmot2, LastUpdated: &now}, {NodeName: &marmot1, LastUpdated: &now}}

	sigA := clusterMemberSignature(statusesA)
	sigB := clusterMemberSignature(statusesB)
	if sigA != sigB {
		t.Fatalf("signature should be order independent: sigA=%s sigB=%s", sigA, sigB)
	}
}
