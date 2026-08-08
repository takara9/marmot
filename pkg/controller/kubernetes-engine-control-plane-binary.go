package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	kubernetesReleaseBaseURL               = "https://dl.k8s.io/release"
	DefaultKubernetesControlPlaneBinaryDir = "/var/lib/marmot/mke/kubernetes"
	kubernetesDownloadTimeout              = 5 * time.Minute
	kubernetesDownloadMaxBytes             = 512 * 1024 * 1024
)

var kubernetesControlPlaneBinaries = []string{"kube-apiserver", "kube-scheduler", "kube-controller-manager"}
var kubernetesVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.\d+)?$`)
var kubernetesHTTPClient = &http.Client{Timeout: kubernetesDownloadTimeout}

var kubernetesDownload = func(rawURL string) ([]byte, error) {
	response, err := kubernetesHTTPClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", response.StatusCode, rawURL)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, kubernetesDownloadMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > kubernetesDownloadMaxBytes {
		return nil, fmt.Errorf("response for %s exceeds max allowed size (%d bytes)", rawURL, kubernetesDownloadMaxBytes)
	}
	return data, nil
}

func ResolveKubernetesPatchVersion(versionSpec string) (string, error) {
	matches := kubernetesVersionPattern.FindStringSubmatch(strings.TrimSpace(versionSpec))
	if matches == nil {
		return "", fmt.Errorf("invalid Kubernetes version %q; expected v<major>.<minor>[.<patch>]", versionSpec)
	}
	stableURL := fmt.Sprintf("%s/stable-%s.%s.txt", kubernetesReleaseBaseURL, matches[1], matches[2])
	data, err := kubernetesDownload(stableURL)
	if err != nil {
		return "", fmt.Errorf("failed to resolve latest Kubernetes patch release: %w", err)
	}
	resolved := strings.TrimSpace(string(data))
	resolvedMatches := kubernetesVersionPattern.FindStringSubmatch(resolved)
	if resolvedMatches == nil || resolvedMatches[1] != matches[1] || resolvedMatches[2] != matches[2] {
		return "", fmt.Errorf("invalid Kubernetes stable version %q for %s.%s", resolved, matches[1], matches[2])
	}
	return resolved, nil
}

func KubernetesControlPlaneBinaryPath(cacheDir, resolvedVersion, binaryName string) (string, error) {
	if !isKubernetesControlPlaneBinary(binaryName) {
		return "", fmt.Errorf("unsupported Kubernetes control plane binary %q", binaryName)
	}
	if kubernetesVersionPattern.FindStringSubmatch(strings.TrimSpace(resolvedVersion)) == nil {
		return "", fmt.Errorf("invalid resolved Kubernetes version %q", resolvedVersion)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", fmt.Errorf("unsupported CPU architecture for Kubernetes download: %s", runtime.GOARCH)
	}
	return filepath.Join(cacheDir, resolvedVersion+"-linux-"+runtime.GOARCH, binaryName), nil
}

func EnsureKubernetesControlPlaneBinaries(cacheDir, versionSpec string) (string, map[string]string, error) {
	resolvedVersion, err := ResolveKubernetesPatchVersion(versionSpec)
	if err != nil {
		return "", nil, err
	}
	paths, err := EnsureResolvedKubernetesControlPlaneBinaries(cacheDir, resolvedVersion)
	if err != nil {
		return "", nil, err
	}
	return resolvedVersion, paths, nil
}

func EnsureResolvedKubernetesControlPlaneBinaries(cacheDir, resolvedVersion string) (map[string]string, error) {
	paths := make(map[string]string, len(kubernetesControlPlaneBinaries))
	for _, binaryName := range kubernetesControlPlaneBinaries {
		path, err := ensureKubernetesControlPlaneBinary(cacheDir, resolvedVersion, binaryName)
		if err != nil {
			return nil, err
		}
		paths[binaryName] = path
	}
	return paths, nil
}

func ensureKubernetesControlPlaneBinary(cacheDir, resolvedVersion, binaryName string) (string, error) {
	path, err := KubernetesControlPlaneBinaryPath(cacheDir, resolvedVersion, binaryName)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
		return path, nil
	}
	binaryURL := fmt.Sprintf("%s/%s/bin/linux/%s/%s", kubernetesReleaseBaseURL, resolvedVersion, runtime.GOARCH, binaryName)
	binaryData, err := kubernetesDownload(binaryURL)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", binaryName, err)
	}
	checksumData, err := kubernetesDownload(binaryURL + ".sha256")
	if err != nil {
		return "", fmt.Errorf("failed to download checksum for %s: %w", binaryName, err)
	}
	wantChecksum := strings.Fields(string(checksumData))
	if len(wantChecksum) == 0 {
		return "", fmt.Errorf("checksum for %s is empty", binaryName)
	}
	actualChecksum := sha256.Sum256(binaryData)
	if !strings.EqualFold(wantChecksum[0], hex.EncodeToString(actualChecksum[:])) {
		return "", fmt.Errorf("checksum mismatch for %s", binaryName)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(binaryData); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	removeTmp = false
	return path, nil
}

func isKubernetesControlPlaneBinary(name string) bool {
	for _, binaryName := range kubernetesControlPlaneBinaries {
		if name == binaryName {
			return true
		}
	}
	return false
}
