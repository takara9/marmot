package controller

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestClusterHasNode(t *testing.T) {
	statuses := []api.HostStatus{
		{NodeName: util.StringPtr("hvc")},
		{NodeName: util.StringPtr("ws1")},
		{},
	}

	tests := []struct {
		name     string
		nodeName string
		want     bool
	}{
		{name: "existing node", nodeName: "hvc", want: true},
		{name: "existing node with spaces", nodeName: " ws1 ", want: true},
		{name: "missing node", nodeName: "not-found", want: false},
		{name: "empty node", nodeName: "", want: false},
		{name: "spaces only", nodeName: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterHasNode(statuses, tt.nodeName)
			if got != tt.want {
				t.Fatalf("clusterHasNode(%q) = %v, want %v", tt.nodeName, got, tt.want)
			}
		})
	}
}

func TestSelectLeastLoadedNode(t *testing.T) {
	activeNodes := []string{"marmot1", "marmot2", "marmot3"}
	loads := map[string]int{
		"marmot1": 2,
		"marmot2": 0,
		"marmot3": 0,
	}

	node, err := selectLeastLoadedNode(activeNodes, loads)
	if err != nil {
		t.Fatalf("selectLeastLoadedNode returned error: %v", err)
	}
	if node != "marmot2" {
		t.Fatalf("selectLeastLoadedNode() = %q, want %q", node, "marmot2")
	}
}

func TestBuildNodeLoads(t *testing.T) {
	activeNodes := []string{"marmot1", "marmot2"}
	statusDeleting := &api.Status{StatusCode: db.SERVER_DELETING}
	statusRunning := &api.Status{StatusCode: db.SERVER_RUNNING}

	servers := []api.Server{
		{Metadata: api.Metadata{Name: "s1", NodeName: util.StringPtr("marmot1")}, Status: statusRunning},
		{Metadata: api.Metadata{Name: "s2", NodeName: util.StringPtr("marmot1")}, Status: statusRunning},
		{Metadata: api.Metadata{Name: "s3", NodeName: util.StringPtr("marmot2")}, Status: statusDeleting},
		{Metadata: api.Metadata{Name: "s4", NodeName: util.StringPtr("other")}, Status: statusRunning},
	}

	loads := buildNodeLoads(activeNodes, servers)
	if loads["marmot1"] != 2 {
		t.Fatalf("buildNodeLoads()[marmot1] = %d, want 2", loads["marmot1"])
	}
	if loads["marmot2"] != 0 {
		t.Fatalf("buildNodeLoads()[marmot2] = %d, want 0", loads["marmot2"])
	}
}

func TestActiveNodeNames(t *testing.T) {
	now := time.Now()
	old := now.Add(-marmotd.ActiveHostThreshold - time.Second)

	statuses := []api.HostStatus{
		{NodeName: util.StringPtr("marmot1"), LastUpdated: &now},
		{NodeName: util.StringPtr("marmot2"), LastUpdated: &now},
		{NodeName: util.StringPtr("stale"), LastUpdated: &old},
		{},
	}

	nodes := activeNodeNames(statuses)
	if len(nodes) != 2 {
		t.Fatalf("activeNodeNames() len = %d, want 2", len(nodes))
	}
}
