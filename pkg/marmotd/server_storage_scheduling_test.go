package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestChooseAssignedNodeName(t *testing.T) {
	t.Run("keeps nodeName empty when request and storage are both unset", func(t *testing.T) {
		node, err := chooseAssignedNodeName("hv-default", nil, "")
		if err != nil {
			t.Fatalf("chooseAssignedNodeName() error = %v", err)
		}
		if node != "" {
			t.Fatalf("chooseAssignedNodeName() = %q, want empty", node)
		}
	})

	t.Run("uses storage node when request node is unset", func(t *testing.T) {
		node, err := chooseAssignedNodeName("hv-default", nil, "hv-storage")
		if err != nil {
			t.Fatalf("chooseAssignedNodeName() error = %v", err)
		}
		if node != "hv-storage" {
			t.Fatalf("chooseAssignedNodeName() = %q, want %q", node, "hv-storage")
		}
	})

	t.Run("keeps requested node when storage node is empty", func(t *testing.T) {
		node, err := chooseAssignedNodeName("hv-default", util.StringPtr("hv-requested"), "")
		if err != nil {
			t.Fatalf("chooseAssignedNodeName() error = %v", err)
		}
		if node != "hv-requested" {
			t.Fatalf("chooseAssignedNodeName() = %q, want %q", node, "hv-requested")
		}
	})

	t.Run("accepts matching requested and storage nodes", func(t *testing.T) {
		node, err := chooseAssignedNodeName("hv-default", util.StringPtr("hv-storage"), "hv-storage")
		if err != nil {
			t.Fatalf("chooseAssignedNodeName() error = %v", err)
		}
		if node != "hv-storage" {
			t.Fatalf("chooseAssignedNodeName() = %q, want %q", node, "hv-storage")
		}
	})

	t.Run("overrides auto-assigned default node with storage node", func(t *testing.T) {
		node, err := chooseAssignedNodeName("hv-default", util.StringPtr("hv-default"), "hv-storage")
		if err != nil {
			t.Fatalf("chooseAssignedNodeName() error = %v", err)
		}
		if node != "hv-storage" {
			t.Fatalf("chooseAssignedNodeName() = %q, want %q", node, "hv-storage")
		}
	})

	t.Run("rejects conflicting requested and storage nodes", func(t *testing.T) {
		_, err := chooseAssignedNodeName("hv-default", util.StringPtr("hv-requested"), "hv-storage")
		if err == nil {
			t.Fatal("chooseAssignedNodeName() error = nil, want conflict error")
		}
	})
}

func volWithNode(nodeName string) *api.Volume {
	return &api.Volume{Metadata: api.Metadata{NodeName: util.StringPtr(nodeName)}}
}

func volWithNodeAndIqn(nodeName, iqn string) *api.Volume {
	return &api.Volume{
		Metadata: api.Metadata{NodeName: util.StringPtr(nodeName)},
		Spec:     api.VolSpec{IscsiTargetIqn: util.StringPtr(iqn)},
	}
}

func TestNodeNameFromResolvedVolumes(t *testing.T) {
	t.Run("returns empty when no volumes", func(t *testing.T) {
		node, err := nodeNameFromResolvedVolumes(nil)
		if err != nil || node != "" {
			t.Fatalf("got (%q, %v), want (\"\", nil)", node, err)
		}
	})

	t.Run("returns node from single volume", func(t *testing.T) {
		node, err := nodeNameFromResolvedVolumes([]*api.Volume{volWithNode("hv1")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "hv1" {
			t.Fatalf("got %q, want %q", node, "hv1")
		}
	})

	t.Run("returns same node when all volumes on same node", func(t *testing.T) {
		vols := []*api.Volume{volWithNode("hv1"), volWithNode("hv1")}
		node, err := nodeNameFromResolvedVolumes(vols)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "hv1" {
			t.Fatalf("got %q, want %q", node, "hv1")
		}
	})

	t.Run("returns error when volumes on different nodes", func(t *testing.T) {
		vols := []*api.Volume{volWithNode("hv1"), volWithNode("hv2")}
		_, err := nodeNameFromResolvedVolumes(vols)
		if err == nil {
			t.Fatal("expected error for volumes on different nodes, got nil")
		}
	})

	t.Run("ignores iscsiTargetIqn volume for node binding", func(t *testing.T) {
		// iSCSI IQN がセットされたボリュームはどのノードからもアクセス可能なので制約しない
		vols := []*api.Volume{volWithNodeAndIqn("hv-iscsi", "iqn.2023-01.com.example:target1")}
		node, err := nodeNameFromResolvedVolumes(vols)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "" {
			t.Fatalf("got %q, want empty (iscsi volume should not constrain node)", node)
		}
	})

	t.Run("local volume determines node; iscsiTargetIqn volume is ignored", func(t *testing.T) {
		vols := []*api.Volume{
			volWithNode("hv1"),
			volWithNodeAndIqn("hv-iscsi", "iqn.2023-01.com.example:target1"),
		}
		node, err := nodeNameFromResolvedVolumes(vols)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "hv1" {
			t.Fatalf("got %q, want %q", node, "hv1")
		}
	})

	t.Run("iscsiTargetIqn volumes on mixed nodes do not cause conflict error", func(t *testing.T) {
		// 2つの iSCSI ボリュームが異なるノードに存在しても、配置制約はかからない
		vols := []*api.Volume{
			volWithNodeAndIqn("hv1", "iqn.2023-01.com.example:target1"),
			volWithNodeAndIqn("hv2", "iqn.2023-01.com.example:target2"),
		}
		node, err := nodeNameFromResolvedVolumes(vols)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "" {
			t.Fatalf("got %q, want empty", node)
		}
	})

	t.Run("skips nil volume", func(t *testing.T) {
		vols := []*api.Volume{nil, volWithNode("hv1")}
		node, err := nodeNameFromResolvedVolumes(vols)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node != "hv1" {
			t.Fatalf("got %q, want %q", node, "hv1")
		}
	})
}
