package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func TestShouldDeleteVolumeForMissingAssignedNode(t *testing.T) {
	hvc := util.StringPtr("hvc")
	ws1 := util.StringPtr("ws1")
	statuses := []api.HostStatus{{NodeName: hvc}}

	tests := []struct {
		name       string
		vol        api.Volume
		statuses   []api.HostStatus
		wantDelete bool
		wantReason string
	}{
		{
			name: "pending volume with missing assigned node does not delete",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_PENDING},
			},
			statuses:   statuses,
			wantDelete: false,
			wantReason: "",
		},
		{
			name: "deleting volume with existing assigned node does not delete",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: hvc},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   statuses,
			wantDelete: false,
			wantReason: "",
		},
		{
			name: "deleting volume with missing assigned node deletes",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   statuses,
			wantDelete: true,
			wantReason: "assigned_node_not_found",
		},
		{
			name: "empty cluster status skips delete to avoid false positives",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   []api.HostStatus{},
			wantDelete: false,
			wantReason: "",
		},
		{
			name: "nil nodeName does not delete",
			vol: api.Volume{
				Metadata: api.Metadata{},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   statuses,
			wantDelete: false,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDelete, gotReason := shouldDeleteVolumeForMissingAssignedNode(tt.vol, tt.statuses)
			if gotDelete != tt.wantDelete {
				t.Fatalf("shouldDeleteVolumeForMissingAssignedNode() = %v, want %v", gotDelete, tt.wantDelete)
			}
			if gotReason != tt.wantReason {
				t.Fatalf("shouldDeleteVolumeForMissingAssignedNode() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestShouldFailVolumeForMissingAssignedNode(t *testing.T) {
	hvc := util.StringPtr("hvc")
	ws1 := util.StringPtr("ws1")
	statuses := []api.HostStatus{{NodeName: hvc}}

	tests := []struct {
		name       string
		vol        api.Volume
		statuses   []api.HostStatus
		wantFail bool
		wantMessage string
	}{
		{
			name: "pending volume with missing assigned node fails",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_PENDING},
			},
			statuses:    statuses,
			wantFail:    true,
			wantMessage: "metadata.nodeName \"ws1\" is not found in cluster",
		},
		{
			name: "provisioning volume with missing assigned node fails",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_PROVISIONING},
			},
			statuses:    statuses,
			wantFail:    true,
			wantMessage: "metadata.nodeName \"ws1\" is not found in cluster",
		},
		{
			name: "deleting volume with missing assigned node does not fail",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:    statuses,
			wantFail:    false,
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFail, gotMessage := shouldFailVolumeForMissingAssignedNode(tt.vol, tt.statuses)
			if gotFail != tt.wantFail {
				t.Fatalf("shouldFailVolumeForMissingAssignedNode() = %v, want %v", gotFail, tt.wantFail)
			}
			if gotMessage != tt.wantMessage {
				t.Fatalf("shouldFailVolumeForMissingAssignedNode() message = %q, want %q", gotMessage, tt.wantMessage)
			}
		})
	}
}
