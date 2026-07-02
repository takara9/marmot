package marmotd

import (
	"strings"
	"testing"
)

func TestHostBridgeIPAMMissingConfigError(t *testing.T) {
	err := hostBridgeIPAMMissingConfigError("host-bridge")
	if err == nil {
		t.Fatalf("hostBridgeIPAMMissingConfigError() returned nil")
	}
	msg := err.Error()
	mustContain := []string{
		"host-bridge",
		"host-bridge-ip-net-addr",
		"host-bridge-ip-addr-start",
		"host-bridge-ip-addr-end",
	}
	for _, part := range mustContain {
		if !strings.Contains(msg, part) {
			t.Fatalf("error message %q does not contain %q", msg, part)
		}
	}
}
