package controller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// buildFakeEtcdArchive は "<tagLinuxArch>/etcd" というエントリのみを含むtar.gzを生成する。
func buildFakeEtcdArchive(t *testing.T, tag, arch, binaryContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	name := fmt.Sprintf("%s-linux-%s/etcd", tag, arch)
	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(binaryContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar WriteHeader() failed: %v", err)
	}
	if _, err := tw.Write([]byte(binaryContent)); err != nil {
		t.Fatalf("tar Write() failed: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close() failed: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close() failed: %v", err)
	}
	return buf.Bytes()
}

func TestEtcdBinaryPath(t *testing.T) {
	path, err := EtcdBinaryPath("/var/lib/marmot/mke/etcd", "3.6.8")
	if err != nil {
		t.Fatalf("EtcdBinaryPath() failed: %v", err)
	}
	want := filepath.Join("/var/lib/marmot/mke/etcd", "v3.6.8-linux-"+runtime.GOARCH, "etcd")
	if path != want {
		t.Fatalf("EtcdBinaryPath() = %q, want %q", path, want)
	}
}

func TestEtcdArchFromGOARCHUnsupported(t *testing.T) {
	if _, err := etcdArchFromGOARCH("riscv64"); err == nil {
		t.Fatalf("expected error for unsupported arch")
	}
}

func TestEnsureEtcdBinaryDownloadsVerifiesAndCaches(t *testing.T) {
	arch, err := etcdArchFromGOARCH(runtime.GOARCH)
	if err != nil {
		t.Skipf("unsupported test host arch: %v", err)
	}

	const version = "3.6.8"
	const tag = "v3.6.8"
	const binaryContent = "#!/bin/sh\necho fake-etcd\n"
	assetName := fmt.Sprintf("etcd-%s-linux-%s.tar.gz", tag, arch)
	archive := buildFakeEtcdArchive(t, tag, arch, binaryContent)
	sum := sha256.Sum256(archive)
	sumsContent := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	origDownload := etcdDownload
	downloadCount := 0
	t.Cleanup(func() { etcdDownload = origDownload })
	etcdDownload = func(url string) ([]byte, error) {
		downloadCount++
		switch {
		case url == fmt.Sprintf("%s/%s/%s", etcdReleaseBaseURL, tag, assetName):
			return archive, nil
		case url == fmt.Sprintf("%s/%s/SHA256SUMS", etcdReleaseBaseURL, tag):
			return []byte(sumsContent), nil
		default:
			return nil, fmt.Errorf("unexpected download URL: %s", url)
		}
	}

	cacheDir := t.TempDir()
	binPath, err := EnsureEtcdBinary(cacheDir, version)
	if err != nil {
		t.Fatalf("EnsureEtcdBinary() failed: %v", err)
	}
	content, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) failed: %v", binPath, err)
	}
	if string(content) != binaryContent {
		t.Fatalf("etcd binary content = %q, want %q", content, binaryContent)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("os.Stat(%q) failed: %v", binPath, err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("etcd binary is not executable: mode=%v", info.Mode())
	}
	if downloadCount != 2 {
		t.Fatalf("etcdDownload call count = %d, want 2", downloadCount)
	}

	// 2回目はキャッシュ済みのためダウンロードを行わないこと(冪等性)
	if _, err := EnsureEtcdBinary(cacheDir, version); err != nil {
		t.Fatalf("EnsureEtcdBinary() second call failed: %v", err)
	}
	if downloadCount != 2 {
		t.Fatalf("etcdDownload was called again on cached binary: count=%d", downloadCount)
	}
}

func TestEnsureEtcdBinaryChecksumMismatch(t *testing.T) {
	arch, err := etcdArchFromGOARCH(runtime.GOARCH)
	if err != nil {
		t.Skipf("unsupported test host arch: %v", err)
	}
	const version = "3.6.8"
	const tag = "v3.6.8"
	assetName := fmt.Sprintf("etcd-%s-linux-%s.tar.gz", tag, arch)
	archive := buildFakeEtcdArchive(t, tag, arch, "content")

	origDownload := etcdDownload
	t.Cleanup(func() { etcdDownload = origDownload })
	etcdDownload = func(url string) ([]byte, error) {
		switch {
		case url == fmt.Sprintf("%s/%s/%s", etcdReleaseBaseURL, tag, assetName):
			return archive, nil
		case url == fmt.Sprintf("%s/%s/SHA256SUMS", etcdReleaseBaseURL, tag):
			return []byte(fmt.Sprintf("%s  %s\n", "0000000000000000000000000000000000000000000000000000000000000000", assetName)), nil
		default:
			return nil, fmt.Errorf("unexpected download URL: %s", url)
		}
	}

	if _, err := EnsureEtcdBinary(t.TempDir(), version); err == nil {
		t.Fatalf("expected checksum mismatch error")
	}
}
