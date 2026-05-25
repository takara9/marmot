package controller

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestCollectActiveHostStatusByNode(t *testing.T) {
	now := time.Now()
	statuses := []api.HostStatus{
		{
			NodeName:    util.StringPtr("marmot1"),
			LastUpdated: util.TimePtr(now.Add(-5 * time.Second)),
		},
		{
			NodeName:    util.StringPtr("marmot2"),
			LastUpdated: util.TimePtr(now.Add(-2 * time.Minute)),
		},
	}

	active := collectActiveHostStatusByNode(statuses, now)
	if len(active) != 1 {
		t.Fatalf("unexpected active count: got %d, want 1", len(active))
	}
	if _, ok := active["marmot1"]; !ok {
		t.Fatalf("expected marmot1 to be active")
	}
}

func TestSelectFailoverHubNode_PrioritizeHostID(t *testing.T) {
	participants := map[string]struct{}{
		"marmot1": {},
		"marmot2": {},
		"marmot3": {},
	}
	activeByNode := map[string]api.HostStatus{
		"marmot1": {HostId: util.StringPtr("0x100")},
		"marmot2": {HostId: util.StringPtr("0x010")},
		"marmot3": {HostId: util.StringPtr("0x200")},
	}

	hub := selectFailoverHubNode(participants, activeByNode)
	if hub != "marmot2" {
		t.Fatalf("unexpected hub node: got %q, want %q", hub, "marmot2")
	}
}

func TestSelectFailoverHubNode_FallbackToNodeNameOrder(t *testing.T) {
	participants := map[string]struct{}{
		"marmot3": {},
		"marmot1": {},
	}
	activeByNode := map[string]api.HostStatus{
		"marmot1": {HostId: util.StringPtr("invalid")},
		"marmot3": {HostId: util.StringPtr("invalid")},
	}

	hub := selectFailoverHubNode(participants, activeByNode)
	if hub != "marmot1" {
		t.Fatalf("unexpected hub node: got %q, want %q", hub, "marmot1")
	}
}
