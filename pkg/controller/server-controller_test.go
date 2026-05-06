package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func TestClusterHasAnyNode(t *testing.T) {
	tests := []struct {
		name     string
		statuses []api.HostStatus
		want     bool
	}{
		{
			name: "has valid node",
			statuses: []api.HostStatus{
				{NodeName: util.StringPtr("hvc")},
			},
			want: true,
		},
		{
			name: "node name with spaces only",
			statuses: []api.HostStatus{
				{NodeName: util.StringPtr("   ")},
				{},
			},
			want: false,
		},
		{
			name: "empty statuses",
			statuses: []api.HostStatus{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clusterHasAnyNode(tt.statuses)
			if got != tt.want {
				t.Fatalf("clusterHasAnyNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldBypassNodeGateForDeletingServer(t *testing.T) {
	deletingStatus := &api.Status{StatusCode: db.SERVER_DELETING}
	runningStatus := &api.Status{StatusCode: db.SERVER_RUNNING}

	statusesWithNode := []api.HostStatus{{NodeName: util.StringPtr("hvc")}}
	emptyStatuses := []api.HostStatus{}

	tests := []struct {
		name       string
		spec       api.Server
		statuses   []api.HostStatus
		wantBypass bool
		wantReason string
	}{
		{
			name:       "not deleting",
			spec:       api.Server{Status: runningStatus},
			statuses:   statusesWithNode,
			wantBypass: false,
			wantReason: "",
		},
		{
			name:       "cluster empty",
			spec:       api.Server{Status: deletingStatus},
			statuses:   emptyStatuses,
			wantBypass: true,
			wantReason: "cluster_nodes_empty",
		},
		{
			name: "assigned node not found",
			spec: api.Server{
				Status: deletingStatus,
				Metadata: &api.Metadata{
					NodeName: util.StringPtr("ws1"),
				},
			},
			statuses:   statusesWithNode,
			wantBypass: true,
			wantReason: "assigned_node_not_found",
		},
		{
			name: "assigned node found",
			spec: api.Server{
				Status: deletingStatus,
				Metadata: &api.Metadata{
					NodeName: util.StringPtr("hvc"),
				},
			},
			statuses:   statusesWithNode,
			wantBypass: false,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBypass, gotReason := shouldBypassNodeGateForDeletingServer(tt.spec, tt.statuses)
			if gotBypass != tt.wantBypass || gotReason != tt.wantReason {
				t.Fatalf("shouldBypassNodeGateForDeletingServer() = (%v, %q), want (%v, %q)", gotBypass, gotReason, tt.wantBypass, tt.wantReason)
			}
		})
	}
}
