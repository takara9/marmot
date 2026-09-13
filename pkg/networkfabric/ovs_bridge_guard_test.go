package networkfabric

import "testing"

func TestIsProtectedBridge(t *testing.T) {
	if !isProtectedBridge("br-int") {
		t.Fatalf("br-int should be protected")
	}
	if !isProtectedBridge("  br-int  ") {
		t.Fatalf("br-int with surrounding whitespace should be protected")
	}
	if isProtectedBridge("br-6ad8b") {
		t.Fatalf("network-specific bridge should not be protected")
	}
	if isProtectedBridge("") {
		t.Fatalf("empty bridge name should not be protected")
	}
}
