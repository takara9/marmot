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

		It("iscsi_server: true を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"iscsi_server":true}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.IscsiServer).To(BeTrue())
		})

		It("iscsi_server が未指定の場合は false (既定値)", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"node_name":"hv1"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.IscsiServer).To(BeFalse())
		})

		It("dns_upstream_allow_cidrs の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"dns_upstream_allow_cidrs":["192.168.1.0/24"," fd00::/64 ",""]}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DNSUpstreamAllowCIDRs).To(Equal([]string{"192.168.1.0/24", "fd00::/64"}))
		})

		It("ceph_enabled の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"ceph_enabled":true}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			_, err = marmotd.LoadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ceph_monitors"))
		})

		It("ceph_enabled が未指定の場合は false (既定値)", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"node_name":"hv1"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephEnabled).To(BeFalse())
		})

		It("Ceph monitors の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"ceph_monitors":["10.1.4.11:6789","10.1.4.12:6789"," 10.1.4.13:6789 "]}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephMonitors).To(Equal([]string{"10.1.4.11:6789", "10.1.4.12:6789", "10.1.4.13:6789"}))
		})

		It("Ceph user と key_file の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"ceph_user":"client.marmot","ceph_key_file":"/etc/ceph/marmot.client.key"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephUser).To(Equal("client.marmot"))
			Expect(cfg.CephKeyFile).To(Equal("/etc/ceph/marmot.client.key"))
		})

		It("Ceph CRUSH rule と pool のマップを読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_crush_rule_by_class": {"hdd":"rule-hdd","ssd":"rule-ssd"},
				"ceph_pool_by_class": {"hdd":"marmot-hdd","ssd":"marmot-ssd","nvme":"marmot-nvme"}
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephCrushRuleByClass).To(Equal(map[string]string{"hdd": "rule-hdd", "ssd": "rule-ssd"}))
			Expect(cfg.CephPoolByClass).To(Equal(map[string]string{"hdd": "marmot-hdd", "ssd": "marmot-ssd", "nvme": "marmot-nvme"}))
		})

		It("Ceph マップの空キーと空値は除外される", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_pool_by_class": {" hdd ":"marmot-hdd","ssd":" ","":"should-be-excluded"," ":"also-excluded"}
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephPoolByClass).To(Equal(map[string]string{"hdd": "marmot-hdd"}))
		})

		It("Ceph マップが未指定の場合は空マップになる", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"node_name":"hv1"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephCrushRuleByClass).NotTo(BeNil())
			Expect(cfg.CephPoolByClass).NotTo(BeNil())
			Expect(cfg.CephCrushRuleByClass).To(HaveLen(0))
			Expect(cfg.CephPoolByClass).To(HaveLen(0))
		})

		It("Ceph 全パラメーターを一度に読み込む", func() {
			dir := GinkgoT().TempDir()
			keyFile := filepath.Join(dir, "marmot.client.key")
			err := os.WriteFile(keyFile, []byte("dummykey"), 0o600)
			Expect(err).NotTo(HaveOccurred())

			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_enabled": true,
				"ceph_monitors": ["10.1.4.11:6789", "10.1.4.12:6789"],
				"ceph_user": "client.marmot",
				"ceph_key_file": "` + keyFile + `",
				"ceph_crush_rule_by_class": {"hdd": "rule-hdd", "ssd": "rule-ssd"},
				"ceph_pool_by_class": {"hdd": "marmot-hdd", "ssd": "marmot-ssd"}
			}`)

			err = os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephEnabled).To(BeTrue())
			Expect(cfg.CephMonitors).To(Equal([]string{"10.1.4.11:6789", "10.1.4.12:6789"}))
			Expect(cfg.CephUser).To(Equal("client.marmot"))
			Expect(cfg.CephKeyFile).To(Equal(keyFile))
			Expect(cfg.CephCrushRuleByClass).To(Equal(map[string]string{"hdd": "rule-hdd", "ssd": "rule-ssd"}))
			Expect(cfg.CephPoolByClass).To(Equal(map[string]string{"hdd": "marmot-hdd", "ssd": "marmot-ssd"}))
		})
	})
})
