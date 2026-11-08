package marmotd

import (
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func leaderTestStatus(nodeName, hostID string, updatedSecondsAgo int) api.HostStatus {
	t := time.Now().Add(-time.Duration(updatedSecondsAgo) * time.Second)
	return api.HostStatus{
		NodeName:    util.StringPtr(nodeName),
		HostId:      util.StringPtr(hostID),
		LastUpdated: &t,
	}
}

func leaderFlags(statuses []api.HostStatus) map[string]bool {
	flags := map[string]bool{}
	for _, s := range statuses {
		flags[*s.NodeName] = s.SchedulerLeader != nil && *s.SchedulerLeader
	}
	return flags
}

func TestAnnotateSchedulerLeaderStatuses(t *testing.T) {
	t.Run("最小HostIdのアクティブノードのみがリーダー", func(t *testing.T) {
		in := []api.HostStatus{
			leaderTestStatus("hv2", "007f0102", 1),
			leaderTestStatus("hv1", "007f0101", 1),
			leaderTestStatus("hv3", "007f0103", 1),
		}
		got := leaderFlags(annotateSchedulerLeaderStatuses(in))
		if !got["hv1"] || got["hv2"] || got["hv3"] {
			t.Fatalf("leader flags = %v, want only hv1", got)
		}
		if in[0].SchedulerLeader != nil {
			t.Fatalf("input must not be modified")
		}
	})

	t.Run("非アクティブなノードはリーダーにならない", func(t *testing.T) {
		in := []api.HostStatus{
			leaderTestStatus("hv1", "007f0101", 120),
			leaderTestStatus("hv2", "007f0102", 1),
		}
		got := leaderFlags(annotateSchedulerLeaderStatuses(in))
		if got["hv1"] || !got["hv2"] {
			t.Fatalf("leader flags = %v, want only hv2", got)
		}
	})

	t.Run("アクティブなノードがなければリーダーなし", func(t *testing.T) {
		in := []api.HostStatus{leaderTestStatus("hv1", "007f0101", 120)}
		got := leaderFlags(annotateSchedulerLeaderStatuses(in))
		if got["hv1"] {
			t.Fatalf("leader flags = %v, want no leader", got)
		}
	})
}
