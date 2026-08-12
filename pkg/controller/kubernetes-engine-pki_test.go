package controller

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineCA", func() {
	It("creates the CA and is idempotent on repeated calls", func() {
		pkiDir := GinkgoT().TempDir()

		certPath, keyPath, err := EnsureKubernetesEngineCA(pkiDir, "demo")
		Expect(err).NotTo(HaveOccurred())
		Expect(certFileExists(certPath)).To(BeTrue())
		Expect(certFileExists(keyPath)).To(BeTrue())

		firstCert, err := os.ReadFile(certPath)
		Expect(err).NotTo(HaveOccurred())

		// 2回目の呼び出しは再生成せず同じ内容を返す(冪等)こと
		certPath2, keyPath2, err := EnsureKubernetesEngineCA(pkiDir, "demo")
		Expect(err).NotTo(HaveOccurred())
		Expect(certPath2).To(Equal(certPath))
		Expect(keyPath2).To(Equal(keyPath))
		secondCert, err := os.ReadFile(certPath2)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(secondCert)).To(Equal(string(firstCert)))
	})

	It("rejects invalid cluster names", func() {
		pkiDir := GinkgoT().TempDir()
		_, _, err := EnsureKubernetesEngineCA(pkiDir, " ")
		Expect(err).To(HaveOccurred())
		_, _, err = EnsureKubernetesEngineCA(pkiDir, "../etc")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("IssueKubernetesEngineCertificate", func() {
	It("issues server and client certificates that verify against the CA and are cached", func() {
		pkiDir := GinkgoT().TempDir()
		caCertPath, _, err := EnsureKubernetesEngineCA(pkiDir, "demo")
		Expect(err).NotTo(HaveOccurred())
		caCertPEM, err := os.ReadFile(caCertPath)
		Expect(err).NotTo(HaveOccurred())
		caPool := x509.NewCertPool()
		Expect(caPool.AppendCertsFromPEM(caCertPEM)).To(BeTrue())

		serverCertPath, serverKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
			Name:        "kube-apiserver",
			CommonName:  "kube-apiserver",
			Usage:       KubernetesEngineCertUsageServer,
			DNSNames:    []string{"localhost"},
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(certFileExists(serverCertPath)).To(BeTrue())
		Expect(certFileExists(serverKeyPath)).To(BeTrue())
		assertCertVerifiesAgainstCA(caPool, serverCertPath, x509.ExtKeyUsageServerAuth)

		clientCertPath, clientKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
			Name:       "kubelet-node1",
			CommonName: "system:node:node1",
			Usage:      KubernetesEngineCertUsageClient,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(certFileExists(clientCertPath)).To(BeTrue())
		Expect(certFileExists(clientKeyPath)).To(BeTrue())
		assertCertVerifiesAgainstCA(caPool, clientCertPath, x509.ExtKeyUsageClientAuth)

		// 同名での再発行は既存ファイルを再利用する(冪等)こと
		firstCert, err := os.ReadFile(serverCertPath)
		Expect(err).NotTo(HaveOccurred())
		serverCertPath2, serverKeyPath2, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
			Name:       "kube-apiserver",
			CommonName: "kube-apiserver",
			Usage:      KubernetesEngineCertUsageServer,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(serverCertPath2).To(Equal(serverCertPath))
		Expect(serverKeyPath2).To(Equal(serverKeyPath))
		secondCert, err := os.ReadFile(serverCertPath2)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(secondCert)).To(Equal(string(firstCert)))
	})

	It("requires an existing CA", func() {
		pkiDir := GinkgoT().TempDir()
		_, _, err := IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
			Name:       "kube-apiserver",
			CommonName: "kube-apiserver",
			Usage:      KubernetesEngineCertUsageServer,
		})
		Expect(err).To(HaveOccurred())
	})

	It("rejects an invalid certificate request name", func() {
		pkiDir := GinkgoT().TempDir()
		_, _, err := EnsureKubernetesEngineCA(pkiDir, "demo")
		Expect(err).NotTo(HaveOccurred())
		_, _, err = IssueKubernetesEngineCertificate(pkiDir, "demo", KubernetesEngineCertRequest{
			Name:       "../evil",
			CommonName: "evil",
			Usage:      KubernetesEngineCertUsageServer,
		})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("withKubernetesEnginePkiLock", func() {
	It("serializes concurrent callers", func() {
		lockPath := filepath.Join(GinkgoT().TempDir(), ".test.lock")
		started := make(chan struct{})
		release := make(chan struct{})
		acquired := make(chan struct{})
		errCh := make(chan error, 2)

		go func() {
			errCh <- withKubernetesEnginePkiLock(lockPath, func() error {
				close(started)
				<-release
				return nil
			})
		}()

		<-started

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- withKubernetesEnginePkiLock(lockPath, func() error {
				close(acquired)
				return nil
			})
		}()

		select {
		case <-acquired:
			Fail("second caller acquired lock before first caller released it")
		case <-time.After(100 * time.Millisecond):
		}

		close(release)
		wg.Wait()

		select {
		case <-acquired:
		case <-time.After(time.Second):
			Fail("second caller did not acquire lock after release")
		}

		for i := 0; i < 2; i++ {
			Expect(<-errCh).NotTo(HaveOccurred())
		}
	})
})

func assertCertVerifiesAgainstCA(caPool *x509.CertPool, certPath string, usage x509.ExtKeyUsage) {
	certPEM, err := os.ReadFile(certPath)
	Expect(err).NotTo(HaveOccurred())
	block, _ := pem.Decode(certPEM)
	Expect(block).NotTo(BeNil())
	cert, err := x509.ParseCertificate(block.Bytes)
	Expect(err).NotTo(HaveOccurred())
	opts := x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{usage},
	}
	_, err = cert.Verify(opts)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("KubernetesEnginePkiDirLayout", func() {
	It("places CA files under the expected pki directory", func() {
		pkiDir := GinkgoT().TempDir()
		certPath, keyPath, err := EnsureKubernetesEngineCA(pkiDir, "demo")
		Expect(err).NotTo(HaveOccurred())
		wantDir := filepath.Join(pkiDir, "demo")
		Expect(filepath.Dir(certPath)).To(Equal(wantDir))
		Expect(filepath.Dir(keyPath)).To(Equal(wantDir))
	})
})
