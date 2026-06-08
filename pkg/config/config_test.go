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
	It("decodes lowerCamel keys like apiVersion and sourceUrl", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, "image.yaml")
		content := "apiVersion: v1\nkind: Image\nmetadata:\n  name: ubuntu22.04\nspec:\n  sourceUrl: http://hmc/ubuntu-22.04-server-cloudimg-amd64.img\n"

		err := os.WriteFile(configPath, []byte(content), 0o600)
		Expect(err).NotTo(HaveOccurred())

		var conf api.Image
		err = config.ReadYamlConfig(configPath, &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.ApiVersion).To(Equal("v1"))
		Expect(conf.Kind).To(Equal("Image"))
		Expect(conf.Metadata.Name).To(Equal("ubuntu22.04"))
		Expect(conf.Spec.SourceUrl).NotTo(BeNil())
		Expect(*conf.Spec.SourceUrl).To(Equal("http://hmc/ubuntu-22.04-server-cloudimg-amd64.img"))
	})

	It("reads a YAML config from a file", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, "server.yaml")
		content := "metadata:\n  name: file-server\nspec:\n  cpu: 2\n"

		err := os.WriteFile(configPath, []byte(content), 0o600)
		Expect(err).NotTo(HaveOccurred())

		var conf api.Server
		err = config.ReadYamlConfig(configPath, &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Metadata).NotTo(BeNil())
		Expect(conf.Metadata.Name).NotTo(BeNil())
		Expect(conf.Metadata.Name).To(Equal("file-server"))
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
			_, _ = w.Write([]byte("metadata:\n  name: url-server\nspec:\n  memory: 2048\n"))
		}))
		defer server.Close()

		var conf api.Server
		err := config.ReadYamlConfig(server.URL+"/server.yaml", &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Metadata).NotTo(BeNil())
		Expect(conf.Metadata.Name).NotTo(BeNil())
		Expect(conf.Metadata.Name).To(Equal("url-server"))
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

	It("reads metadata.comment from a YAML config", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, "server-metadata.yaml")
		content := "metadata:\n  name: metadata-server\n  comment: created-from-metadata\n"

		err := os.WriteFile(configPath, []byte(content), 0o600)
		Expect(err).NotTo(HaveOccurred())

		var conf api.Server
		err = config.ReadYamlConfig(configPath, &conf)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Metadata).NotTo(BeNil())
		Expect(conf.Metadata.Comment).NotTo(BeNil())
		Expect(*conf.Metadata.Comment).To(Equal("created-from-metadata"))
	})
})
