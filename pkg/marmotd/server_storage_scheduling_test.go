package marmotd

import (
	"testing"

	"github.com/takara9/marmot/pkg/util"
)

func TestChooseAssignedNodeName(t *testing.T) {
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
