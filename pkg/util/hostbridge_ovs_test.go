//go:build linux
// +build linux

package util

import (
	"os"
	"path/filepath"
	"testing"
)

// writeOVSVsctl creates a fake ovs-vsctl script in dir and returns dir.
// exitCode is the exit code the script will return.
func writeOVSVsctl(t *testing.T, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "ovs-vsctl")
	content := "#!/bin/sh\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write fake ovs-vsctl: %v", err)
	}
	return dir
}

// itoa converts an int to its decimal string representation without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 3)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	old := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+":"+old); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Setenv("PATH", old); err != nil {
			t.Fatalf("failed to restore PATH: %v", err)
		}
	})
}

func TestIsOVSBridge_EmptyName(t *testing.T) {
	if got := IsOVSBridge(""); got {
		t.Error("IsOVSBridge(\"\") = true, want false")
	}
}

func TestIsOVSBridge_OVSVsctlNotFound(t *testing.T) {
	// Use a temp dir with no ovs-vsctl binary so exec.ErrNotFound is triggered.
	dir := t.TempDir()
	prependPATH(t, dir)

	if got := IsOVSBridge("br0"); got {
		t.Error("IsOVSBridge with missing ovs-vsctl = true, want false")
	}
}

func TestIsOVSBridge_BrExistsFails(t *testing.T) {
	// ovs-vsctl exits with non-zero (bridge not managed by OVS).
	dir := writeOVSVsctl(t, 2)
	prependPATH(t, dir)

	if got := IsOVSBridge("br0"); got {
		t.Error("IsOVSBridge with failing br-exists = true, want false")
	}
}

func TestIsOVSBridge_BrExistsSucceeds(t *testing.T) {
	// ovs-vsctl exits with 0 (bridge is managed by OVS).
	dir := writeOVSVsctl(t, 0)
	prependPATH(t, dir)

	if got := IsOVSBridge("ovsbr0"); !got {
		t.Error("IsOVSBridge with successful br-exists = false, want true")
	}
}
