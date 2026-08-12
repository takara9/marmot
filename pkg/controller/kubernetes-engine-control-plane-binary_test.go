package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesControlPlaneBinaries", func() {
	It("resolves a Kubernetes minor version to its latest patch version", func() {
		origDownload := kubernetesDownload
		DeferCleanup(func() { kubernetesDownload = origDownload })
		var gotURL string
		kubernetesDownload = func(rawURL string) ([]byte, error) {
			gotURL = rawURL
			return []byte("v1.36.7\n"), nil
		}

		for _, input := range []string{"1.36", "v1.36", "v1.36.2"} {
			resolved, err := ResolveKubernetesPatchVersion(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(gotURL).To(Equal(kubernetesReleaseBaseURL + "/stable-1.36.txt"))
			Expect(resolved).To(Equal("v1.36.7"))
		}
	})

	It("downloads control-plane binaries and caches them across calls", func() {
		origDownload := kubernetesDownload
		DeferCleanup(func() { kubernetesDownload = origDownload })
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

		cacheDir := GinkgoT().TempDir()
		resolved, paths, err := EnsureKubernetesControlPlaneBinaries(cacheDir, "v1.36.2")
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal("v1.36.7"))
		Expect(paths).To(HaveLen(3))
		for name, path := range paths {
			info, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred(), "binary %s not written", name)
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o755)))
		}

		_, _, err = EnsureKubernetesControlPlaneBinaries(cacheDir, "v1.36")
		Expect(err).NotTo(HaveOccurred())
		Expect(binaryDownloads).To(Equal(3))
	})

	It("rejects a checksum mismatch", func() {
		origDownload := kubernetesDownload
		DeferCleanup(func() { kubernetesDownload = origDownload })
		kubernetesDownload = func(rawURL string) ([]byte, error) {
			if strings.HasSuffix(rawURL, ".txt") {
				return []byte("v1.36.7"), nil
			}
			if strings.HasSuffix(rawURL, ".sha256") {
				return []byte(strings.Repeat("0", 64)), nil
			}
			return []byte("binary"), nil
		}
		_, _, err := EnsureKubernetesControlPlaneBinaries(GinkgoT().TempDir(), "v1.36")
		Expect(err).To(HaveOccurred())
	})
})
