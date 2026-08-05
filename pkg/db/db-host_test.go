package db

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
)

func TestResolveHostStatusCreationTimestamp(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	incomingCreation := now.Add(-48 * time.Hour)
	existingCreation := now.Add(-24 * time.Hour)
	existingUpdated := now.Add(-6 * time.Hour)

	tests := []struct {
		name     string
		incoming api.HostStatus
		existing *api.HostStatus
		want     time.Time
	}{
		{
			name:     "prefers incoming creation timestamp",
			incoming: api.HostStatus{CreationTimeStamp: &incomingCreation},
			existing: &api.HostStatus{CreationTimeStamp: &existingCreation, LastUpdated: &existingUpdated},
			want:     incomingCreation,
		},
		{
			name:     "reuses existing creation timestamp",
			incoming: api.HostStatus{},
			existing: &api.HostStatus{CreationTimeStamp: &existingCreation, LastUpdated: &existingUpdated},
			want:     existingCreation,
		},
		{
			name:     "falls back to existing last updated for upgraded records",
			incoming: api.HostStatus{},
			existing: &api.HostStatus{LastUpdated: &existingUpdated},
			want:     existingUpdated,
		},
		{
			name:     "uses current time when no prior timestamps exist",
			incoming: api.HostStatus{},
			existing: nil,
			want:     now,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := resolveHostStatusCreationTimestamp(tc.incoming, tc.existing, now)
			if got == nil {
				t.Fatal("resolveHostStatusCreationTimestamp() returned nil")
			}
			if !got.Equal(tc.want) {
				t.Fatalf("resolveHostStatusCreationTimestamp() = %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}