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
			Expect(cfg.CephVolumeOperationTimeoutSeconds).To(Equal(120))
			Expect(cfg.DefaultUnderlayInterface).To(Equal(""))
			Expect(cfg.SessionIdleTimeout).To(Equal("1h"))
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
			content := []byte(`{"image_create_from_vm_timeout_seconds":120,"image_create_from_url_timeout_seconds":900,"image_download_timeout_seconds":600,"image_resize_timeout_seconds":180,"image_delete_timeout_seconds":45,"ceph_volume_operation_timeout_seconds":75}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ImageCreateFromVMTimeoutSeconds).To(Equal(120))
			Expect(cfg.ImageCreateFromURLTimeoutSeconds).To(Equal(900))
			Expect(cfg.ImageDownloadTimeoutSeconds).To(Equal(600))
			Expect(cfg.ImageResizeTimeoutSeconds).To(Equal(180))
			Expect(cfg.ImageDeleteTimeoutSeconds).To(Equal(45))
			Expect(cfg.CephVolumeOperationTimeoutSeconds).To(Equal(75))
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

		It("session_idle_timeout の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"session_idle_timeout":"3d"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.SessionIdleTimeout).To(Equal("3d"))
		})

		It("session_idle_timeout が不正な単位の場合はエラーになる", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"session_idle_timeout":"30x"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			_, err = marmotd.LoadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session_idle_timeout unit"))
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

		It("dns_client_allow_cidrs の設定値を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"dns_client_allow_cidrs":["192.168.1.0/24"," fd00::/64 ",""]}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DNSUpstreamAllowCIDRs).To(Equal([]string{"192.168.1.0/24", "fd00::/64"}))
		})

		It("dns_upstream_allow_cidrs (旧キー) も読み込める", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"dns_upstream_allow_cidrs":["192.168.2.0/24"]}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DNSUpstreamAllowCIDRs).To(Equal([]string{"192.168.2.0/24"}))
		})

		It("dns_client_allow_ciders (互換キー) も読み込める", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"dns_client_allow_ciders":["192.168.3.0/24"]}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DNSUpstreamAllowCIDRs).To(Equal([]string{"192.168.3.0/24"}))
		})

		It("複数キー同時指定時は正規キーを優先する", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"dns_client_allow_cidrs":["10.0.0.0/24"],
				"dns_client_allow_ciders":["10.1.0.0/24"],
				"dns_upstream_allow_cidrs":["10.2.0.0/24"]
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DNSUpstreamAllowCIDRs).To(Equal([]string{"10.0.0.0/24"}))
		})

		It("正規キーがない場合は誤記キーを旧キーより優先する", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"dns_client_allow_ciders":["10.1.0.0/24"],
				"dns_upstream_allow_cidrs":["10.2.0.0/24"]
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DNSUpstreamAllowCIDRs).To(Equal([]string{"10.1.0.0/24"}))
		})

		It("ceph_enabled の設定値を読み込む", func() {
			if _, err := os.Stat(marmotd.DefaultCephConfPath); err != nil {
				Skip("default ceph.conf is required")
			}
			if _, err := os.Stat(marmotd.DefaultCephKeyringPath); err != nil {
				Skip("default ceph keyring is required")
			}

			dir := GinkgoT().TempDir()

			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"ceph_enabled":true}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephEnabled).To(BeTrue())
		})

		It("ceph_enabled=true かつ ceph.conf 未配置の場合はエラーになる", func() {
			if _, err := os.Stat(marmotd.DefaultCephConfPath); err == nil {
				Skip("default ceph.conf exists; missing-file scenario cannot be simulated without env override")
			}

			dir := GinkgoT().TempDir()

			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"ceph_enabled":true}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			_, err = marmotd.LoadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ceph conf file"))
		})

		It("ceph keyring file にディレクトリを指定した場合はエラーになる", func() {
			Skip("ceph keyring path override test was removed with env override deprecation")
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

		It("Ceph CRUSH rule と pool の設定を読み込む", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_crush_rule_by_class": {"hdd":"rule-hdd","ssd":"rule-ssd"},
				"ceph_pool_by_class": [
					{"storageClass":"hdd","pool":"marmot-hdd"},
					{"storageClass":"ssd","pool":"marmot-ssd"},
					{"storageClass":"nvme","pool":"marmot-nvme"}
				]
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephCrushRuleByClass).To(Equal(map[string]string{"hdd": "rule-hdd", "ssd": "rule-ssd"}))
			Expect(cfg.CephPoolByClass).To(Equal(map[string]string{"hdd": "marmot-hdd", "ssd": "marmot-ssd", "nvme": "marmot-nvme"}))
		})

		It("Ceph pool 設定の空キーと空値は除外される", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_pool_by_class": [
					{"storageClass":" hdd ","pool":"marmot-hdd"},
					{"storageClass":"ssd","pool":" "},
					{"storageClass":"","pool":"should-be-excluded"},
					{"storageClass":" ","pool":"also-excluded"}
				]
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephPoolByClass).To(Equal(map[string]string{"hdd": "marmot-hdd"}))
		})

		It("Ceph pool 設定に旧 map 形式を指定した場合はエラーになる", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_pool_by_class": {"hdd":"marmot-hdd"}
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			_, err = marmotd.LoadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ceph_pool_by_class"))
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

		It("Ceph 設定（有効化+classマップ）を一度に読み込む", func() {
			if _, err := os.Stat(marmotd.DefaultCephConfPath); err != nil {
				Skip("default ceph.conf is required")
			}
			if _, err := os.Stat(marmotd.DefaultCephKeyringPath); err != nil {
				Skip("default ceph keyring is required")
			}

			dir := GinkgoT().TempDir()

			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{
				"ceph_enabled": true,
				"ceph_crush_rule_by_class": {"hdd": "rule-hdd", "ssd": "rule-ssd"},
				"ceph_pool_by_class": [
					{"storageClass":"hdd","pool":"marmot-hdd"},
					{"storageClass":"ssd","pool":"marmot-ssd"}
				]
			}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := marmotd.LoadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CephEnabled).To(BeTrue())
			Expect(cfg.CephCrushRuleByClass).To(Equal(map[string]string{"hdd": "rule-hdd", "ssd": "rule-ssd"}))
			Expect(cfg.CephPoolByClass).To(Equal(map[string]string{"hdd": "marmot-hdd", "ssd": "marmot-ssd"}))
		})
	})

	Describe("session_idle_timeout overflow guard", func() {
		It("rejects extremely large hours", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"session_idle_timeout":"9223372036854775807h"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			_, err = marmotd.LoadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("<= 7d"))
		})

		It("rejects extremely large days", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "marmotd.json")
			content := []byte(`{"session_idle_timeout":"9223372036854775807d"}`)

			err := os.WriteFile(path, content, 0o644)
			Expect(err).NotTo(HaveOccurred())

			_, err = marmotd.LoadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("<= 7d"))
		})
	})
})
