package networkfabric

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestParseInterfaceOfport_Ready(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("\"12\"")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatalf("expected ready=true")
	}
	if ofport != 12 {
		t.Fatalf("unexpected ofport: got %d, want 12", ofport)
	}
}

func TestParseInterfaceOfport_NotReadyMinusOne(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatalf("expected ready=false")
	}
	if ofport != 0 {
		t.Fatalf("unexpected ofport: got %d, want 0", ofport)
	}
}

func TestParseInterfaceOfport_NotReadyZero(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatalf("expected ready=false")
	}
	if ofport != 0 {
		t.Fatalf("unexpected ofport: got %d, want 0", ofport)
	}
}

func TestParseInterfaceOfport_InvalidText(t *testing.T) {
	_, _, err := parseInterfaceOfport("[]")
	if err == nil {
		t.Fatalf("expected error for invalid input")
	}
}

func TestParseInterfaceOfport_TrimmedQuotedValue(t *testing.T) {
	ofport, ready, err := parseInterfaceOfport("  \"7\"  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatalf("expected ready=true")
	}
	if ofport != 7 {
		t.Fatalf("unexpected ofport: got %d, want 7", ofport)
	}
}

func TestResolveOverlayVNI_UsesExplicitSpecValue(t *testing.T) {
	vnet := &api.VirtualNetwork{
		Metadata: api.Metadata{Name: "webs-net"},
		Spec: api.VirtualNetworkSpec{
			Vni: util.IntPtrInt(1234),
		},
	}

	vni, err := resolveOverlayVNI(vnet)
	if err != nil {
		t.Fatalf("resolveOverlayVNI() returned error: %v", err)
	}
	if vni != 1234 {
		t.Fatalf("resolveOverlayVNI() = %d, want %d", vni, 1234)
	}
}

func TestResolveOverlayVNI_DerivesStableValueFromName(t *testing.T) {
	vnetA := &api.VirtualNetwork{Metadata: api.Metadata{Name: "webs-net"}}
	vnetB := &api.VirtualNetwork{Metadata: api.Metadata{Name: "webs-net"}}

	vniA, err := resolveOverlayVNI(vnetA)
	if err != nil {
		t.Fatalf("resolveOverlayVNI(vnetA) returned error: %v", err)
	}
	vniB, err := resolveOverlayVNI(vnetB)
	if err != nil {
		t.Fatalf("resolveOverlayVNI(vnetB) returned error: %v", err)
	}

	if vniA == 0 {
		t.Fatalf("derived vni must not be zero")
	}
	if vniA != vniB {
		t.Fatalf("derived vni must be stable: got %d and %d", vniA, vniB)
	}
}

func TestResolveOverlayVNI_DifferentNamesYieldDifferentValues(t *testing.T) {
	vnetA := &api.VirtualNetwork{Metadata: api.Metadata{Name: "webs-net"}}
	vnetB := &api.VirtualNetwork{Metadata: api.Metadata{Name: "db-net"}}

	vniA, err := resolveOverlayVNI(vnetA)
	if err != nil {
		t.Fatalf("resolveOverlayVNI(vnetA) returned error: %v", err)
	}
	vniB, err := resolveOverlayVNI(vnetB)
	if err != nil {
		t.Fatalf("resolveOverlayVNI(vnetB) returned error: %v", err)
	}

	if vniA == vniB {
		t.Fatalf("different network names should not share default vni: got %d", vniA)
	}
}
