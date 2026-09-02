package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("downloadImageFromHeadWithContext", func() {
	It("sends the auth token as a Bearer Authorization header", func() {
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
		Expect(gotAuthz).To(Equal("Bearer test-token"))

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
