package config_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
)

var _ = Describe("ReadYamlConfig", func() {
	It("reads a YAML config from a file", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, "server.yaml")
		content := "Metadata:\n  name: file-server\nSpec:\n  cpu: 2\n"

		err := os.WriteFile(configPath, []byte(content), 0o600)
		Expect(err).NotTo(HaveOccurred())

		var conf api.Server
		err = config.ReadYamlConfig(configPath, &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Metadata).NotTo(BeNil())
		Expect(conf.Metadata.Name).NotTo(BeNil())
		Expect(*conf.Metadata.Name).To(Equal("file-server"))
		Expect(conf.Spec).NotTo(BeNil())
		Expect(conf.Spec.Cpu).NotTo(BeNil())
		Expect(*conf.Spec.Cpu).To(Equal(2))
	})

	It("reads a YAML config from a URL", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/server.yaml" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte("Metadata:\n  name: url-server\nSpec:\n  memory: 2048\n"))
		}))
		defer server.Close()

		var conf api.Server
		err := config.ReadYamlConfig(server.URL+"/server.yaml", &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Metadata).NotTo(BeNil())
		Expect(conf.Metadata.Name).NotTo(BeNil())
		Expect(*conf.Metadata.Name).To(Equal("url-server"))
		Expect(conf.Spec).NotTo(BeNil())
		Expect(conf.Spec.Memory).NotTo(BeNil())
		Expect(*conf.Spec.Memory).To(Equal(2048))
	})

	It("returns an error when the URL responds with a non-200 status", func() {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()

		var conf api.Server
		err := config.ReadYamlConfig(server.URL+"/missing.yaml", &conf)
		Expect(err).To(HaveOccurred())
	})

	It("returns an error for invalid YAML from a URL", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("name: [unterminated\n"))
		}))
		defer server.Close()

		var conf api.Server
		err := config.ReadYamlConfig(server.URL, &conf)
		Expect(err).To(HaveOccurred())
	})

	It("reads Metadata.comment from a YAML config", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, "server-Metadata.yaml")
		content := "Metadata:\n  name: metadata-server\n  comment: created-from-Metadata\n"

		err := os.WriteFile(configPath, []byte(content), 0o600)
		Expect(err).NotTo(HaveOccurred())

		var conf api.Server
		err = config.ReadYamlConfig(configPath, &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Metadata).NotTo(BeNil())
		Expect(conf.Metadata.Comment).NotTo(BeNil())
		Expect(*conf.Metadata.Comment).To(Equal("created-from-Metadata"))
	})
})
