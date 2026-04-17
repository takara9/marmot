package marmotd_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("VolumeGroupConfig", func() {
	Describe("LoadConfig", func() {
		It("os_volume_group と data_volume_group が未指定なら既定値を補完する", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"node_name":"hv9","etcd_url":"http://127.0.0.1:12379"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.OSVolumeGroup).To(Equal(db.DefaultOSVolumeGroup))
			Expect(cfg.DataVolumeGroup).To(Equal(db.DefaultDataVolumeGroup))
		})

		It("os_volume_group と data_volume_group の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"os_volume_group":"sysvg","data_volume_group":"datavg"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.OSVolumeGroup).To(Equal("sysvg"))
			Expect(cfg.DataVolumeGroup).To(Equal("datavg"))
		})
	})
})
