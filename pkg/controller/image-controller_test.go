package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/takara9/marmot/pkg/marmotd"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("downloadImageFromHeadWithContext", func() {
	It("uses HTTPS when marmotd is configured for TLS", func() {
		orig := marmotd.CurrentConfig()
		marmotd.SetRuntimeConfig(&marmotd.MarmotdConfig{
			APIListenAddr: "0.0.0.0:9443",
			TLSCertFile:   "/etc/marmot/certs/server.crt",
			TLSKeyFile:    "/etc/marmot/certs/server.key",
		})
		defer marmotd.SetRuntimeConfig(orig)

		Expect(buildHeadImageDownloadURL("10.0.0.2", "image-123")).To(Equal("https://10.0.0.2:9443/api/v1/image/image-123/qcow2"))
	})

	It("sends the auth token as a ****** header", func() {
		var gotAuthz string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuthz = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("qcow2-bytes"))
		}))
		defer server.Close()

		destPath := filepath.Join(GinkgoT().TempDir(), "image.qcow2")
		err := downloadImageFromHeadWithContext(context.Background(), server.URL, destPath, "test-token")
		Expect(err).NotTo(HaveOccurred())
		Expect(gotAuthz).To(Equal("******"))

		data, err := os.ReadFile(destPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("qcow2-bytes"))
	})

	It("returns an error when the head node rejects the request as unauthorized", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		destPath := filepath.Join(GinkgoT().TempDir(), "image.qcow2")
		err := downloadImageFromHeadWithContext(context.Background(), server.URL, destPath, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("401"))
	})
})
