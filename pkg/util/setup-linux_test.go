package util_test

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Linux セットアップ", Ordered, func() {

	BeforeAll(func(ctx0 SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		// テスト用のqcow2ボリュームの準備
		cmd := exec.Command("cp", "/var/lib/marmot/volumes/jammy-server-cloudimg-amd64.img", "/var/lib/marmot/volumes/test-linux-qcow2.img")
		err := cmd.Run()
		Expect(err).To(BeNil())

		// テスト用のLVMボリュームをスナップショットで準備
		err = exec.Command("lvcreate", "-L", "1G", "-s", "-n", "lvos_test1", "/dev/vg1/lvos_temp").Run()
		Expect(err).To(BeNil())

		// テスト用のLVMボリュームをスナップショットで準備
		err = exec.Command("lvcreate", "-L", "1G", "-s", "-n", "lvos_test2", "/dev/vg1/lvos_temp").Run()
		Expect(err).To(BeNil())

	})

	AfterAll(func(ctx0 SpecContext) {
		// テスト環境のクリーンアップ
	})

	Context("qcow2 ブートデバイス設定のテスト", func() {
		// テスト用のサーバースペックを定義
		testSpec := api.Server{
			Id: "a123456",
			Metadata: &api.Metadata{
				Name: util.StringPtr("test-linux"),
			},
			Spec: &api.VmSpec{
				BootVolume: &api.Volume{
					Id: "test-linux-boot",
					Spec: &api.VolSpec{
						Type: util.StringPtr("qcow2"),
						Path: util.StringPtr("/var/lib/marmot/volumes/test-linux-qcow2.img"),
					},
				},
			},
		}
		var mountPoint string
		var nbdDev string
		var err error

		It("Linuxセットアップのテスト", func() {
			err := util.SetupLinux(testSpec)
			Expect(err).To(BeNil())
		})

		It("チェックのためのマウント", func() {
			mountPoint, nbdDev, err = util.MountVolume(*testSpec.Spec.BootVolume)
			Expect(err).To(BeNil())
		})

		It("ホスト名設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/hostname")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(*testSpec.Metadata.Name))
		})

		It("Linux hostid設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/machine-id")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(testSpec.Id))
		})

		It("Netplan設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/netplan/00-nic.yaml")
			Expect(err).To(BeNil())
			fmt.Println(string(data))
		})

		It("アンマウントLinuxホスト名設定のチェック", func() {
			fmt.Println("Unmounting volume...", mountPoint, nbdDev)
			util.UnMountVolume(*testSpec.Spec.BootVolume, mountPoint, nbdDev)
		})
	})

	Context("lvm ブートデバイス設定のテスト", func() {
		// テスト用のサーバースペックを定義
		testSpec := api.Server{
			Id: "b123456",
			Metadata: &api.Metadata{
				Name: util.StringPtr("test-linux-lvm"),
			},
			Spec: &api.VmSpec{
				BootVolume: &api.Volume{
					Id:            "test-linux-boot2",
					Spec: &api.VolSpec{
						Type:          util.StringPtr("lvm"),
						Path:          util.StringPtr("/dev/mapper/vg1-lvos_test1"),
						VolumeGroup:   util.StringPtr("vg1"),
						LogicalVolume: util.StringPtr("lvos_test1"),
					},
				},
			},
		}
		var mountPoint string
		var nbdDev string
		var err error

		It("Linuxセットアップのテスト", func() {
			err := util.SetupLinux(testSpec)
			Expect(err).To(BeNil())
		})

		It("チェックのためのマウント", func() {
			mountPoint, nbdDev, err = util.MountVolume(*testSpec.Spec.BootVolume)
			Expect(err).To(BeNil())
		})

		It("ホスト名設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/hostname")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(*testSpec.Metadata.Name))
		})

		It("Linux hostid設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/machine-id")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(testSpec.Id))
		})

		It("Netplan設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/netplan/00-nic.yaml")
			Expect(err).To(BeNil())
			fmt.Println(string(data))
		})

		It("アンマウントLinuxホスト名設定のチェック", func() {
			fmt.Println("Unmounting volume...", mountPoint, nbdDev)
			err := util.UnMountVolume(*testSpec.Spec.BootVolume, mountPoint, nbdDev)
			Expect(err).To(BeNil())
		})
	})

	Context("複数NIC設定のテスト", func() {
		testSpec := api.Server{
			Id: "c123456",
			Metadata: &api.Metadata{
				Name: util.StringPtr("test-linux-mh"),
			},
			Spec: &api.VmSpec{
				BootVolume: &api.Volume{
					Id:            "test-linux-boot3",
					Spec: &api.VolSpec{
						Type:          util.StringPtr("lvm"),
						Path:          util.StringPtr("/dev/mapper/vg1-lvos_test2"),
						VolumeGroup:   util.StringPtr("vg1"),
						LogicalVolume: util.StringPtr("lvos_test2"),
					},
				},
				Network: &[]api.Network{
					{
						Id: "default",
					},
					{
						Id: "host-bridge",
					},
				},
			},
		}
		var mountPoint string
		var nbdDev string
		var err error

		It("Linuxセットアップのテスト", func() {
			err := util.SetupLinux(testSpec)
			Expect(err).To(BeNil())
		})

		It("チェックのためのマウント", func() {
			mountPoint, nbdDev, err = util.MountVolume(*testSpec.Spec.BootVolume)
			Expect(err).To(BeNil())
		})

		It("ホスト名設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/hostname")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(*testSpec.Metadata.Name))
		})

		It("Linux hostid設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/machine-id")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(testSpec.Id))
		})

		It("Netplan設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/netplan/00-nic.yaml")
			Expect(err).To(BeNil())
			fmt.Println(string(data))
		})

		It("アンマウントLinuxホスト名設定のチェック", func() {
			fmt.Println("Unmounting volume...", mountPoint, nbdDev)
			err := util.UnMountVolume(*testSpec.Spec.BootVolume, mountPoint, nbdDev)
			Expect(err).To(BeNil())
		})
	})

	Context("最大NIC設定のテスト", func() {
		testSpec := api.Server{
			Id: "d123456",
			Metadata: &api.Metadata{
				Name: util.StringPtr("test-linux-mh"),
			},
			Spec: &api.VmSpec{
				BootVolume: &api.Volume{
					Id:            "test-linux-boot4",
					Spec: &api.VolSpec{
						Type:          util.StringPtr("lvm"),
						Path:          util.StringPtr("/dev/mapper/vg1-lvos_test2"),
						VolumeGroup:   util.StringPtr("vg1"),
						LogicalVolume: util.StringPtr("lvos_test2"),
					},
				},
				Network: &[]api.Network{
					{
						Id: "default",
					},
					{
						Id: "host-bridge",
						Nameservers: &api.Nameservers{
							Addresses: &[]string{
								"8.8.8.8",
								"8.8.4.4",
							},
							Search: &[]string{
								"test.com",
								"test2.org",
							},
						},
					},
					{
						Id:      "host-bridge",
						Address: util.StringPtr("1.2.3.4"),
						Netmask: util.StringPtr("24"),
						Routes: &[]api.Route{
							{
								To:  util.StringPtr("default"),
								Via: util.StringPtr("1.2.3.1"),
							},
						},
					},
					{
						Id:      "host-bridge",
						Address: util.StringPtr("2001:db8::1"),
						Netmask: util.StringPtr("64"),
						Routes: &[]api.Route{
							{
								To:  util.StringPtr("2001:db8::/64"),
								Via: util.StringPtr("2001:db8::ff"),
							},
						},
						Nameservers: &api.Nameservers{
							Addresses: &[]string{
								"2001:4860:4860::8888",
								"2001:4860:4860::8844",
							},
							Search: &[]string{
								"test.com",
								"test2.org",
							},
						},
					},
					{
						Id:    "host-bridge",
						Dhcp4: util.BoolPtr(false),
						Dhcp6: util.BoolPtr(false),
					},
					{
						Id: "host-bridge",
					},
				},
			},
		}
		var mountPoint string
		var nbdDev string
		var err error

		It("Linuxセットアップのテスト", func() {
			err := util.SetupLinux(testSpec)
			Expect(err).To(BeNil())
		})

		It("チェックのためのマウント", func() {
			mountPoint, nbdDev, err = util.MountVolume(*testSpec.Spec.BootVolume)
			Expect(err).To(BeNil())
		})

		It("ホスト名設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/hostname")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(*testSpec.Metadata.Name))
		})

		It("Linux hostid設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/machine-id")
			Expect(err).To(BeNil())
			Expect(string(data)).To(Equal(testSpec.Id))
		})

		It("Netplan設定のチェック", func() {
			data, err := os.ReadFile(mountPoint + "/etc/netplan/00-nic.yaml")
			Expect(err).To(BeNil())
			fmt.Println(string(data))
		})

		It("アンマウントLinuxホスト名設定のチェック", func() {
			fmt.Println("Unmounting volume...", mountPoint, nbdDev)
			err := util.UnMountVolume(*testSpec.Spec.BootVolume, mountPoint, nbdDev)
			Expect(err).To(BeNil())
		})
	})
})
