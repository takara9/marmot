package controller

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResourceIDFromPlaybookPath(t *testing.T) {
	got := resourceIDFromPlaybookPath("/var/lib/marmot/ansible-playbooks/gateway-gw-123.yaml", "gateway-")
	if got != "gw-123" {
		t.Fatalf("resourceIDFromPlaybookPath() = %q, want %q", got, "gw-123")
	}
}

func TestAnsiblePlaybookLogPath_UsesResourceNameAndID(t *testing.T) {
	oldDir := ansiblePlaybookLogDirPath
	ansiblePlaybookLogDirPath = t.TempDir()
	t.Cleanup(func() {
		ansiblePlaybookLogDirPath = oldDir
	})

	path, err := ansiblePlaybookLogPath("gateway", "gw-123")
	if err != nil {
		t.Fatalf("ansiblePlaybookLogPath() failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("gateway-gw-123.log")) {
		t.Fatalf("ansiblePlaybookLogPath() = %q, want suffix %q", path, filepath.Join("gateway-gw-123.log"))
	}
}
