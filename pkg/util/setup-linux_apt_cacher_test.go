//go:build linux
// +build linux

package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAptCacherNGProxyConfig(t *testing.T) {
	mountPoint := t.TempDir()

	if err := writeAptCacherNGProxyConfig(mountPoint); err != nil {
		t.Fatalf("writeAptCacherNGProxyConfig() error = %v", err)
	}

	proxyFile := filepath.Join(mountPoint, "etc/apt/apt.conf.d/95marmot-apt-cacher-ng")
	data, err := os.ReadFile(proxyFile)
	if err != nil {
		t.Fatalf("failed to read written proxy config: %v", err)
	}

	want := "Acquire::http::Proxy \"http://10.245.0.1:3142\";\n"
	if string(data) != want {
		t.Fatalf("proxy config = %q, want %q", string(data), want)
	}
}
