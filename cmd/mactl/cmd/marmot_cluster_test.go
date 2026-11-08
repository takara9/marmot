package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
)

func TestPrintHostClusterShowsLeaderColumn(t *testing.T) {
	hv1, hv2 := "hv1", "hv2"
	leader := true
	notLeader := false

	out := captureStdoutForIDTest(t, func() {
		printHostCluster([]api.HostStatus{
			{NodeName: &hv2, SchedulerLeader: &notLeader},
			{NodeName: &hv1, SchedulerLeader: &leader},
		})
	})

	if !strings.Contains(out, "LEADER") {
		t.Fatalf("stdout = %q, want LEADER header", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout = %q, want header + 2 rows", out)
	}
	if !strings.HasPrefix(lines[1], "hv1") || !strings.Contains(lines[1], "*") {
		t.Fatalf("row = %q, want hv1 marked with *", lines[1])
	}
	if strings.Contains(lines[2], "*") {
		t.Fatalf("row = %q, want hv2 not marked", lines[2])
	}
}

func TestPrintHostClusterUsesAgeColumn(t *testing.T) {
	nodeName := "hv1"
	hostID := "007f0101"
	ipAddress := "10.1.0.11"
	createdAt := time.Now().Add(-26 * time.Hour)

	out := captureStdoutForIDTest(t, func() {
		printHostCluster([]api.HostStatus{{
			NodeName:          &nodeName,
			HostId:            &hostID,
			IpAddress:         &ipAddress,
			CreationTimeStamp: &createdAt,
		}})
	})

	if !strings.Contains(out, "AGE") {
		t.Fatalf("stdout = %q, want AGE header", out)
	}
	if strings.Contains(out, "UPDATED") {
		t.Fatalf("stdout = %q, want UPDATED header removed", out)
	}
	if !strings.Contains(out, "1d") {
		t.Fatalf("stdout = %q, want creation age text", out)
	}
}