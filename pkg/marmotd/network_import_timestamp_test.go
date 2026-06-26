package marmotd

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func TestShouldPreserveSystemNetworkCreationTimestamp(t *testing.T) {
	if !shouldPreserveSystemNetworkCreationTimestamp("default") {
		t.Fatalf("default should be treated as creation timestamp preserved network")
	}
	if !shouldPreserveSystemNetworkCreationTimestamp("host-bridge") {
		t.Fatalf("host-bridge should be treated as creation timestamp preserved network")
	}
	if shouldPreserveSystemNetworkCreationTimestamp("app-net") {
		t.Fatalf("app-net should not be treated as creation timestamp preserved network")
	}
}

func TestMergeImportedNetworkPreservingCreation(t *testing.T) {
	createdAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 26, 9, 30, 0, 0, time.UTC)

	existing := api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata: api.Metadata{
			Id:       "abcde",
			Name:     "default",
			NodeName: util.StringPtr("node-a"),
			Uuid:     util.StringPtr("old-uuid"),
		},
		Spec: api.VirtualNetworkSpec{
			BridgeName: util.StringPtr("virbr0"),
		},
		Status: &api.Status{
			CreationTimeStamp:   util.TimePtr(createdAt),
			LastUpdateTimeStamp: util.TimePtr(createdAt),
			StatusCode:          db.NETWORK_PENDING,
			Status:              util.StringPtr(db.NetworkStatus[db.NETWORK_PENDING]),
		},
	}

	imported := api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata: api.Metadata{
			Name: "default",
			Uuid: util.StringPtr("new-uuid"),
		},
		Spec: api.VirtualNetworkSpec{
			BridgeName: util.StringPtr("virbr0"),
		},
	}

	merged := mergeImportedNetworkPreservingCreation(existing, imported, "node-a", now)

	if merged.Status == nil {
		t.Fatalf("merged status should not be nil")
	}
	if merged.Status.CreationTimeStamp == nil {
		t.Fatalf("creation timestamp should be preserved")
	}
	if !merged.Status.CreationTimeStamp.Equal(createdAt) {
		t.Fatalf("creation timestamp changed: got %v want %v", *merged.Status.CreationTimeStamp, createdAt)
	}
	if merged.Status.LastUpdateTimeStamp == nil {
		t.Fatalf("last update timestamp should be set")
	}
	if !merged.Status.LastUpdateTimeStamp.Equal(now) {
		t.Fatalf("last update timestamp mismatch: got %v want %v", *merged.Status.LastUpdateTimeStamp, now)
	}
	if merged.Status.StatusCode != db.NETWORK_ACTIVE {
		t.Fatalf("status code mismatch: got %d want %d", merged.Status.StatusCode, db.NETWORK_ACTIVE)
	}
	if merged.Status.Status == nil || *merged.Status.Status != db.NetworkStatus[db.NETWORK_ACTIVE] {
		t.Fatalf("status mismatch: got %v want %s", merged.Status.Status, db.NetworkStatus[db.NETWORK_ACTIVE])
	}
	if merged.Metadata.Uuid == nil || *merged.Metadata.Uuid != "new-uuid" {
		t.Fatalf("uuid should be updated from import")
	}
}
