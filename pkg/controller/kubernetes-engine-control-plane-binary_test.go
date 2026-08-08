package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestResolveKubernetesPatchVersion(t *testing.T) {
	origDownload := kubernetesDownload
	t.Cleanup(func() { kubernetesDownload = origDownload })
	kubernetesDownload = func(rawURL string) ([]byte, error) {
		if rawURL != kubernetesReleaseBaseURL+"/stable-1.36.txt" {
			t.Fatalf("unexpected URL: %s", rawURL)
		}
		return []byte("v1.36.7\n"), nil
	}

	for _, input := range []string{"1.36", "v1.36", "v1.36.2"} {
		resolved, err := ResolveKubernetesPatchVersion(input)
		if err != nil {
			t.Fatalf("ResolveKubernetesPatchVersion(%q) failed: %v", input, err)
		}
		if resolved != "v1.36.7" {
			t.Fatalf("resolved = %q, want %q", resolved, "v1.36.7")
		}
	}
}

func TestEnsureKubernetesControlPlaneBinariesDownloadsAndCaches(t *testing.T) {
	origDownload := kubernetesDownload
	t.Cleanup(func() { kubernetesDownload = origDownload })
	binaryDownloads := 0
	kubernetesDownload = func(rawURL string) ([]byte, error) {
		if strings.HasSuffix(rawURL, "stable-1.36.txt") {
			return []byte("v1.36.7\n"), nil
		}
		for _, binaryName := range kubernetesControlPlaneBinaries {
			if strings.HasSuffix(rawURL, "/"+binaryName) {
				binaryDownloads++
				return []byte("binary-" + binaryName), nil
			}
			if strings.HasSuffix(rawURL, "/"+binaryName+".sha256") {
				sum := sha256.Sum256([]byte("binary-" + binaryName))
				return []byte(hex.EncodeToString(sum[:]) + "\n"), nil
			}
		}
		return nil, fmt.Errorf("unexpected URL %s", rawURL)
	}

	cacheDir := t.TempDir()
	resolved, paths, err := EnsureKubernetesControlPlaneBinaries(cacheDir, "v1.36.2")
	if err != nil {
		t.Fatalf("EnsureKubernetesControlPlaneBinaries() failed: %v", err)
	}
	if resolved != "v1.36.7" || len(paths) != 3 {
		t.Fatalf("resolved=%q paths=%v", resolved, paths)
	}
	for name, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("binary %s not written: %v", name, err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("binary %s mode = %o, want 755", name, info.Mode().Perm())
		}
	}

	if _, _, err := EnsureKubernetesControlPlaneBinaries(cacheDir, "v1.36"); err != nil {
		t.Fatalf("cached EnsureKubernetesControlPlaneBinaries() failed: %v", err)
	}
	if binaryDownloads != 3 {
		t.Fatalf("binary downloads = %d, want 3", binaryDownloads)
	}
}

func TestEnsureKubernetesControlPlaneBinariesRejectsChecksumMismatch(t *testing.T) {
	origDownload := kubernetesDownload
	t.Cleanup(func() { kubernetesDownload = origDownload })
	kubernetesDownload = func(rawURL string) ([]byte, error) {
		if strings.HasSuffix(rawURL, ".txt") {
			return []byte("v1.36.7"), nil
		}
		if strings.HasSuffix(rawURL, ".sha256") {
			return []byte(strings.Repeat("0", 64)), nil
		}
		return []byte("binary"), nil
	}
	if _, _, err := EnsureKubernetesControlPlaneBinaries(t.TempDir(), "v1.36"); err == nil {
		t.Fatalf("expected checksum mismatch, got nil")
	}
}
