package controller

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EnsureKubernetesEngineControlPlaneAssets", func() {
	It("issues API server certs and scheduler/controller-manager kubeconfigs", func() {
		assets, err := EnsureKubernetesEngineControlPlaneAssets(GinkgoT().TempDir(), GinkgoT().TempDir(), "demo", "172.16.90.100", 6443)
		Expect(err).NotTo(HaveOccurred())

		certPEM, err := os.ReadFile(assets.APIServerCertPath)
		Expect(err).NotTo(HaveOccurred())
		block, _ := pem.Decode(certPEM)
		Expect(block).NotTo(BeNil())
		cert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(containsIP(cert.IPAddresses, "172.16.90.100")).To(BeTrue())
		Expect(containsIP(cert.IPAddresses, "127.0.0.1")).To(BeTrue())
		Expect(containsString(cert.DNSNames, "kubernetes.default.svc")).To(BeTrue())

		for _, path := range []string{assets.SchedulerKubeconfigPath, assets.ControllerManagerConfigPath} {
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("https://172.16.90.100:6443"))
		}
		Expect(certFileExists(assets.ServiceAccountPublicKeyPath)).To(BeTrue())
		Expect(certFileExists(assets.ServiceAccountPrivateKeyPath)).To(BeTrue())
	})
})

func containsIP(values []net.IP, want string) bool {
	for _, value := range values {
		if value.String() == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
