package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/config"
)

var _ = Describe("Marmot endpoint config", func() {
	It("normalizes comments when reading legacy config", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, ".marmot")
		content := "current: 0\nendpoints:\n  - http://localhost:8750\n  - http://hv1:8750\n"

		err := os.WriteFile(configPath, []byte(content), 0o600)
		Expect(err).NotTo(HaveOccurred())

		cfg, err := config.ReadMarmotConfig(configPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.EndpointComments).To(HaveLen(2))
		Expect(cfg.EndpointComment(0)).To(Equal(""))
		Expect(cfg.EndpointComment(1)).To(Equal(""))
	})

	It("keeps endpoint and comment indexes aligned on write", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, ".marmot")

		cfg := &config.MarmotConfig{
			Current:          0,
			Endpoints:        []string{"http://localhost:8750"},
			EndpointComments: []string{"comment-a", "extra-comment"},
		}

		err := config.WriteMarmotConfig(configPath, cfg)
		Expect(err).NotTo(HaveOccurred())

		loaded, err := config.ReadMarmotConfig(configPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Endpoints).To(HaveLen(1))
		Expect(loaded.EndpointComments).To(HaveLen(1))
		Expect(loaded.EndpointComment(0)).To(Equal("comment-a"))
	})
})
