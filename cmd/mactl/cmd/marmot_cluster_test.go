package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
)

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