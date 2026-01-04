package virt_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"

	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("LXCコンテナサーバー生成から終了までのライフサイクル", func() {
	var dom1 *libvirtxml.Domain // 仮想マシンの定義の格納先
	//var dom2 *libvirtxml.Domain // 仮想マシンの定義の格納先
	var l *virt.LibVirtEp
	var err error

	Context("定義作成テスト", func() {
		var xs virt.LxcSpec
		It("LibVirtエンドポイントの作成", func() {
			xs.DomainType = "lxc"
			xs.UUID = "c02f10de-50df-4cf3-a4f4-16cfcaaec2e0"
			xs.Name = "lxc-test-1"
			xs.RAM = 2048 * 1024 // MB
			xs.VCPUs = 1
			xs.Machine = "pc-q35-4.2"
			xs.FileSystemSpecs = []virt.FileSystemSpec{
				{SourceDir: "/var/lib/lxc/rootfs/lxc-test-1", TargetDir: "/", Access: "passthrough"},
				{SourceDir: "/var/lib/lxc/shared-data", TargetDir: "/mnt/shared-data", Access: "passthrough"},
			}
			xs.Nets = []virt.NetSpec{
				{
					MAC:     "52:54:00:19:ae:50",
					Network: "host-bridge",
					PortID:  "87525ba8-c378-4123-8deb-2ce8c8ac6051",
					Bridge:  "virbr0",
					Target:  "vnet2",
					Alias:   "net0",
					Bus:     1,
				},
			}
			dom1 = virt.MakeLxcDefinition(xs)
			xml, err := dom1.Marshal()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("-----------------------------")
			fmt.Println("LXCコンテナのドメイン定義:", string(xml))
		})

		It("LibVirtエンドポイントの作成", func() {
			l, err = virt.NewLibVirtEp("lxc:///system")
			Expect(err).NotTo(HaveOccurred())
		})

		It("LXCコンテナのドメインの定義-1", func() {
			err := l.DefineAndStartVM(*dom1)
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち", func() {
			time.Sleep(3 * time.Second)
		})

		It("ドメインのリスト取得", func() {
			nameList, err := l.ListDomains()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Active VM List:", nameList)
			Expect(len(nameList)).To(BeNumerically(">", 0))
			for _, name := range nameList {
				GinkgoWriter.Println("Found VM:", name)
			}
		})

		It("ドメインの削除", func() {
			hostname1 := "lxc-test-1"
			err := l.DeleteDomain(hostname1)
			Expect(err).NotTo(HaveOccurred())
			err = util.RemoveChannelDir(hostname1)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
