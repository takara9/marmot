package controller

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

func TestIsKubernetesEngineInGracePeriod(t *testing.T) {
	withinDelay := time.Now().Add(-5 * time.Second)
	expiredDelay := time.Now().Add(-(KUBERNETES_ENGINE_DELETION_DELAY + time.Second))

	tests := []struct {
		name string
		ke   api.KubernetesEngine
		want bool
	}{
		{
			name: "no status: not in grace period",
			ke:   api.KubernetesEngine{},
			want: false,
		},
		{
			name: "nil DeletionTimeStamp: not in grace period",
			ke:   api.KubernetesEngine{Status: &api.Status{StatusCode: db.KUBERNETES_ENGINE_RUNNING}},
			want: false,
		},
		{
			name: "timestamp within delay: record is in grace period and must be preserved",
			ke: api.KubernetesEngine{
				Status: &api.Status{
					StatusCode:        db.KUBERNETES_ENGINE_DELETING,
					DeletionTimeStamp: &withinDelay,
				},
			},
			want: true,
		},
		{
			name: "timestamp past delay: grace period expired and record may be hard-deleted",
			ke: api.KubernetesEngine{
				Status: &api.Status{
					StatusCode:        db.KUBERNETES_ENGINE_DELETING,
					DeletionTimeStamp: &expiredDelay,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKubernetesEngineInGracePeriod(tt.ke)
			if got != tt.want {
				t.Fatalf("isKubernetesEngineInGracePeriod() = %v, want %v", got, tt.want)
			}
		})
	}
}
