package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

type mockNetworkFabric struct {
	ensureBridgeCalls int
	ensureMeshCalls   int
	pruneMeshCalls    int
}

func (m *mockNetworkFabric) EnsureBridge(vnet *api.VirtualNetwork) error {
	m.ensureBridgeCalls++
	return nil
}

func (m *mockNetworkFabric) EnsureOverlayMesh(vnet *api.VirtualNetwork, peers []string) error {
	m.ensureMeshCalls++
	return nil
}

func (m *mockNetworkFabric) PruneOverlayMesh(vnet *api.VirtualNetwork, remainPeers []string) error {
	m.pruneMeshCalls++
	return nil
}

func (m *mockNetworkFabric) DeleteBridge(vnet *api.VirtualNetwork) error {
	return nil
}

func (m *mockNetworkFabric) GetBridgeStatus(vnet *api.VirtualNetwork) (bool, int, error) {
	return true, 0, nil
}

func TestEnsureOverlayMeshForNetwork_ManualSkipsAutomaticTunnelOps(t *testing.T) {
	overlayMode := api.Vxlan
	peerPolicy := api.Manual
	bridgeName := "br-test"
	vnet := api.VirtualNetwork{
		Metadata: api.Metadata{Name: "test-net", Id: "abcde"},
		Spec: api.VirtualNetworkSpec{
			OverlayMode: &overlayMode,
			PeerPolicy:  &peerPolicy,
			BridgeName:  util.StringPtr(bridgeName),
		},
	}

	fabric := &mockNetworkFabric{}
	c := &controller{}

	err := c.ensureOverlayMeshForNetwork(fabric, vnet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fabric.ensureBridgeCalls != 1 {
		t.Fatalf("ensureBridge call count mismatch: got %d, want 1", fabric.ensureBridgeCalls)
	}
	if fabric.ensureMeshCalls != 0 {
		t.Fatalf("ensureOverlayMesh should not be called for manual policy: got %d", fabric.ensureMeshCalls)
	}
	if fabric.pruneMeshCalls != 0 {
		t.Fatalf("pruneOverlayMesh should not be called for manual policy: got %d", fabric.pruneMeshCalls)
	}
}
