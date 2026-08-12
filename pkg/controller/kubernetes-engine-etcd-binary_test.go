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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildFakeEtcdArchive は "<tagLinuxArch>/etcd" というエントリのみを含むtar.gzを生成する。
func buildFakeEtcdArchive(tag, arch, binaryContent string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	name := fmt.Sprintf("%s-linux-%s/etcd", tag, arch)
	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(binaryContent)),
	}
	Expect(tw.WriteHeader(hdr)).To(Succeed())
	_, err := tw.Write([]byte(binaryContent))
	Expect(err).NotTo(HaveOccurred())
	Expect(tw.Close()).To(Succeed())
	Expect(gz.Close()).To(Succeed())
	return buf.Bytes()
}

var _ = Describe("EtcdBinary", func() {
	It("builds the etcd binary path from the version and host arch", func() {
		path, err := EtcdBinaryPath("/var/lib/marmot/mke/etcd", "3.6.8")
		Expect(err).NotTo(HaveOccurred())
		want := filepath.Join("/var/lib/marmot/mke/etcd", "v3.6.8-linux-"+runtime.GOARCH, "etcd")
		Expect(path).To(Equal(want))
	})

	It("rejects unsupported GOARCH values", func() {
		_, err := etcdArchFromGOARCH("riscv64")
		Expect(err).To(HaveOccurred())
	})

	It("downloads, verifies and caches the etcd binary", func() {
		arch, err := etcdArchFromGOARCH(runtime.GOARCH)
		if err != nil {
			Skip(fmt.Sprintf("unsupported test host arch: %v", err))
		}

		const version = "3.6.8"
		const tag = "v3.6.8"
		const binaryContent = "#!/bin/sh\necho fake-etcd\n"
		assetName := fmt.Sprintf("etcd-%s-linux-%s.tar.gz", tag, arch)
		archive := buildFakeEtcdArchive(tag, arch, binaryContent)
		sum := sha256.Sum256(archive)
		sumsContent := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

		origDownload := etcdDownload
		downloadCount := 0
		DeferCleanup(func() { etcdDownload = origDownload })
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

		cacheDir := GinkgoT().TempDir()
		binPath, err := EnsureEtcdBinary(cacheDir, version)
		Expect(err).NotTo(HaveOccurred())
		content, err := os.ReadFile(binPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal(binaryContent))
		info, err := os.Stat(binPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm() & 0o100).NotTo(BeZero())
		Expect(downloadCount).To(Equal(2))

		// 2回目はキャッシュ済みのためダウンロードを行わないこと(冪等性)
		_, err = EnsureEtcdBinary(cacheDir, version)
		Expect(err).NotTo(HaveOccurred())
		Expect(downloadCount).To(Equal(2))
	})

	It("rejects a checksum mismatch", func() {
		arch, err := etcdArchFromGOARCH(runtime.GOARCH)
		if err != nil {
			Skip(fmt.Sprintf("unsupported test host arch: %v", err))
		}
		const version = "3.6.8"
		const tag = "v3.6.8"
		assetName := fmt.Sprintf("etcd-%s-linux-%s.tar.gz", tag, arch)
		archive := buildFakeEtcdArchive(tag, arch, "content")

		origDownload := etcdDownload
		DeferCleanup(func() { etcdDownload = origDownload })
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

		_, err = EnsureEtcdBinary(GinkgoT().TempDir(), version)
		Expect(err).To(HaveOccurred())
	})
})
