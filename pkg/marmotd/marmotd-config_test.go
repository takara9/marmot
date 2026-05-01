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
			Expect(cfg.ImageCreateFromVMTimeoutSeconds).To(Equal(600))
			Expect(cfg.ImageCreateFromURLTimeoutSeconds).To(Equal(1800))
			Expect(cfg.ImageDownloadTimeoutSeconds).To(Equal(1800))
			Expect(cfg.ImageResizeTimeoutSeconds).To(Equal(600))
			Expect(cfg.ImageDeleteTimeoutSeconds).To(Equal(120))
			Expect(cfg.DefaultUnderlayInterface).To(Equal(""))
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

		It("image timeout 設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"image_create_from_vm_timeout_seconds":120,"image_create_from_url_timeout_seconds":900,"image_download_timeout_seconds":600,"image_resize_timeout_seconds":180,"image_delete_timeout_seconds":45}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ImageCreateFromVMTimeoutSeconds).To(Equal(120))
			Expect(cfg.ImageCreateFromURLTimeoutSeconds).To(Equal(900))
			Expect(cfg.ImageDownloadTimeoutSeconds).To(Equal(600))
			Expect(cfg.ImageResizeTimeoutSeconds).To(Equal(180))
			Expect(cfg.ImageDeleteTimeoutSeconds).To(Equal(45))
		})

		It("default_underlay_interface の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"default_underlay_interface":"enp2s0"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DefaultUnderlayInterface).To(Equal("enp2s0"))
		})
	})
})
