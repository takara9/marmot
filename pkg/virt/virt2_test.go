// テストコードを作成することが第一優先となる
package virt_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"

	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("仮想サーバー生成から終了までのライフサイクル", func() {
	var dom1 *libvirtxml.Domain // 仮想マシンの定義の格納先
	var dom2 *libvirtxml.Domain // 仮想マシンの定義の格納先
	var l *virt.LibVirtEp
	var err error

	// 入力パラメータ
	var vs virt.VmSpec
	var xml1 string
	var xml2 string
	var hostname1 string = "vm-test-1"
	var hostname2 string = "vm-test-2"
	var cpus uint = 2
	var ramMB uint = 4096
	var bootdiskpath string = "/var/lib/marmot/volumes/jammy-server-cloudimg-amd64.img"

	Context("仮想マシンのドメイン作成1", func() {
		var err error
		vs.UUID = uuid.New().String()
		vs.Name = hostname1
		vs.RAM = ramMB * 1024 // MB
		vs.CountVCPU = cpus
		vs.Machine = "pc-q35-4.2"

		vs.DiskSpecs = []virt.DiskSpec{
			{"vda", "/dev/vg1/oslv", 3, "raw"},
			{"vdb", "/dev/vg1/lvdata", 10, "raw"},
			{"vdc", "/var/lib/marmot/volumes/data-vol-1.qcow2", 11, "qcow2"},
		}
		channelFile := "org.qemu.guest_agent.0"
		channelPath, err := util.CreateChannelDir(hostname1)
		Expect(err).NotTo(HaveOccurred())

		mac, err := util.GenerateRandomMAC()
		Expect(err).NotTo(HaveOccurred())
		vs.Nets = []virt.NetSpec{
			{
				MAC:     mac.String(),
				Network: "default",
				PortID:  uuid.New().String(),
				Bridge:  "virbr0",
				Target:  "vnet2",
				Alias:   "net0",
				Bus:     1,
			},
		}
		vs.ChannelSpecs = []virt.ChannelSpec{
			{"unix", channelPath + "/" + channelFile, channelFile, "channel0", 1},
			{"spicevmc", "", "com.redhat.spice.0", "channel1", 2},
		}
		vs.Clocks = []virt.ClockSpec{
			{"rtc", "catchup", ""},
			{"pit", "delay", ""},
			{"hpet", "", "no"},
		}

		dom1 = virt.CreateDomainXML(vs)
		xml1, err = dom1.Marshal()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println("Generated libvirt XML:\n", xml1)
	})

	Context("仮想マシンのドメイン作成2", func() {
		var err error
		vs.UUID = uuid.New().String()
		vs.Name = hostname2
		vs.RAM = ramMB * 1024 // MB
		vs.CountVCPU = cpus
		vs.Machine = "pc-q35-4.2"

		vs.DiskSpecs = []virt.DiskSpec{
			{"vda", bootdiskpath, 3, "qcow2"},
			{"vdb", "/var/lib/marmot/volumes/data-vol-2.qcow2", 10, "qcow2"},
		}
		channelFile := "org.qemu.guest_agent.0"
		channelPath, err := util.CreateChannelDir(hostname2)
		Expect(err).NotTo(HaveOccurred())

		mac, err := util.GenerateRandomMAC()
		Expect(err).NotTo(HaveOccurred())
		vs.Nets = []virt.NetSpec{
			{
				MAC:     mac.String(),
				Network: "default",
				PortID:  uuid.New().String(),
				Bridge:  "virbr0",
				Target:  "vnet2",
				Alias:   "net0",
				Bus:     1,
			},
		}
		vs.ChannelSpecs = []virt.ChannelSpec{
			{"unix", channelPath + "/" + channelFile, channelFile, "channel0", 1},
			{"spicevmc", "", "com.redhat.spice.0", "channel1", 2},
		}
		vs.Clocks = []virt.ClockSpec{
			{"rtc", "catchup", ""},
			{"pit", "delay", ""},
			{"hpet", "", "no"},
		}

		dom2 = virt.CreateDomainXML(vs)
		xml2, err = dom2.Marshal()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println("Generated libvirt XML:\n", xml2)
	})

	It("LibVirtエンドポイントの生成", func() {
		l, err = virt.NewLibVirtEp("qemu:///system")
		Expect(err).NotTo(HaveOccurred())
	})

	It("仮想マシンのドメインの定義-1", func() {
		err := l.DefineAndStartVM(*dom1)
		Expect(err).NotTo(HaveOccurred())
	})

	It("仮想マシンのドメインの定義-2", func() {
		err := l.DefineAndStartVM(*dom2)
		Expect(err).NotTo(HaveOccurred())
	})

	It("時間待ち", func() {
		time.Sleep(3 * time.Second)
	})

	It("ドメインのリスト取得", func() {
		Eventually(func(g Gomega) {
			nameList, err := l.ListDomains()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Active VM List:", nameList)
			Expect(len(nameList)).To(BeNumerically(">", 0))
			for _, name := range nameList {
				if name == hostname1 {
					GinkgoWriter.Println("Found VM:", name)
					return
				}
			}
		}).Should(Succeed())
	})

	It("ドメインの削除", func() {
		err := l.DeleteDomain(hostname1)
		Expect(err).NotTo(HaveOccurred())
		err = util.RemoveChannelDir(hostname1)
		Expect(err).NotTo(HaveOccurred())
	})

	It("ドメインの削除", func() {
		err := l.DeleteDomain(hostname2)
		Expect(err).NotTo(HaveOccurred())
		err = util.RemoveChannelDir(hostname2)
		Expect(err).NotTo(HaveOccurred())
	})

})

// 他テストスペック

// LVM

// インタフェース

// データボリューム
