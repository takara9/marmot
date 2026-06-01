package controller

import (
	"fmt"
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

func TestReconcileOVNClusterBootstrapFromStatuses_OneNode(t *testing.T) {
	now := time.Now()
	statuses := []api.HostStatus{
		{
			NodeName:    util.StringPtr("marmot1"),
			HostId:      util.StringPtr("0x10"),
			IpAddress:   util.StringPtr("172.16.10.2"),
			LastUpdated: util.TimePtr(now),
		},
	}

	called := false
	var gotLeaderIP string
	var gotLocalIP string
	err := reconcileOVNClusterBootstrapFromStatuses("marmot1", statuses, now, func(leaderIP, localIP string) error {
		called = true
		gotLeaderIP = leaderIP
		gotLocalIP = localIP
		return nil
	})
	if err != nil {
		t.Fatalf("reconcileOVNClusterBootstrapFromStatuses returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected bootstrap configure to be called")
	}
	if gotLeaderIP != "172.16.10.2" || gotLocalIP != "172.16.10.2" {
		t.Fatalf("unexpected bootstrap ips: leader=%q local=%q", gotLeaderIP, gotLocalIP)
	}
}

func TestReconcileOVNClusterBootstrapFromStatuses_SelectsClusterLeader(t *testing.T) {
	now := time.Now()
	statuses := []api.HostStatus{
		{
			NodeName:    util.StringPtr("marmot1"),
			HostId:      util.StringPtr("0x30"),
			IpAddress:   util.StringPtr("172.16.10.2"),
			LastUpdated: util.TimePtr(now),
		},
		{
			NodeName:    util.StringPtr("marmot2"),
			HostId:      util.StringPtr("0x10"),
			IpAddress:   util.StringPtr("172.16.10.3"),
			LastUpdated: util.TimePtr(now),
		},
	}

	called := false
	var gotLeaderIP string
	err := reconcileOVNClusterBootstrapFromStatuses("marmot1", statuses, now, func(leaderIP, localIP string) error {
		called = true
		gotLeaderIP = leaderIP
		if localIP != "172.16.10.2" {
			return fmt.Errorf("unexpected local ip %q", localIP)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reconcileOVNClusterBootstrapFromStatuses returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected bootstrap configure to be called")
	}
	if gotLeaderIP != "172.16.10.3" {
		t.Fatalf("unexpected leader ip: got %q want %q", gotLeaderIP, "172.16.10.3")
	}
}

func TestReconcileOVNClusterBootstrapFromStatuses_LocalNodeMissing(t *testing.T) {
	now := time.Now()
	statuses := []api.HostStatus{
		{
			NodeName:    util.StringPtr("marmot2"),
			HostId:      util.StringPtr("0x10"),
			IpAddress:   util.StringPtr("172.16.10.3"),
			LastUpdated: util.TimePtr(now),
		},
	}

	called := false
	err := reconcileOVNClusterBootstrapFromStatuses("marmot1", statuses, now, func(leaderIP, localIP string) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatalf("expected error when local node has no active host status")
	}
	if called {
		t.Fatalf("bootstrap configure should not be called when local node is missing")
	}
}
