package marmotd_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("MKEConfig", func() {
	Describe("LoadMKEConfig", func() {
		It("cmd/marmotd/testdata/mke.json を読み込む", func() {
			cfg, err := marmotd.LoadMKEConfig("../../cmd/marmotd/testdata/mke.json")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.KubernetesVersion).To(Equal("v1.36.2"))
			Expect(cfg.ContainerdVersion).To(Equal("2.3.1"))
			Expect(cfg.EtcdVersion).To(Equal("3.6.8"))
			Expect(cfg.CNIVersion).To(Equal("0.4.0"))
			Expect(cfg.DefaultCNIType).To(Equal("bridge"))
			Expect(cfg.RuncVersion).To(Equal("1.4.0"))
		})

		It("ファイルが存在しない場合はデフォルト値を返す", func() {
			cfg, err := marmotd.LoadMKEConfig("/path/does/not/exist/mke.json")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.KubernetesVersion).To(Equal("v1.36.2"))
			Expect(cfg.DefaultCNIType).To(Equal("bridge"))
		})

		It("一部フィールドのみ指定された場合はデフォルト値で補完する", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "mke.json")
			content := []byte(`{"kubernetes_version":"v1.99.0"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadMKEConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.KubernetesVersion).To(Equal("v1.99.0"))
			Expect(cfg.DefaultCNIType).To(Equal("bridge"))
			Expect(cfg.RuncVersion).To(Equal("1.4.0"))
		})
	})
})
