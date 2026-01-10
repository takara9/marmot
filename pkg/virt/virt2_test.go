// テストコードを作成することが第一優先となる
package virt_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"

	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("仮想サーバー生成から終了までのライフサイクル", func() {
	var dom *libvirtxml.Domain // 仮想マシンの定義の格納先
	var l *virt.LibVirtEp
	var err error

	// 入力パラメータ
	var vs virt.VmSpec
	var xml string
	var hostname string = "vm-test-1"
	var cpus uint = 4
	var ramMB uint = 16384
	//var bootdiskpath string = "/var/lib/libvirt/images/ubuntu22.04.qcow2"
	//var bootdiskpath string = "/var/lib/libvirt/images/jammy-server-cloudimg-amd64.img"

	Context("Lifecycle of LibVirt", func() {
		// 仮想マシン定義の作成
		var err error
		vs.UUID = uuid.New().String()
		vs.Name = hostname
		vs.RAM = ramMB * 1024 // MB
		vs.CountVCPU = cpus
		vs.Machine = "pc-q35-4.2"

		vs.DiskSpecs = []virt.DiskSpec{
			//{"vda", bootdiskpath, 3, "qcow2"},
			{"vda", "/dev/vg1/lvos_test", 3, "raw"},
		}
		channelFile := "org.qemu.guest_agent.0"
		channelPath, err := util.CreateChannelDir(hostname)
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

		dom = virt.CreateDomainXML(vs)
		xml, err = dom.Marshal()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println("Generated libvirt XML:\n", xml)
	})

	It("LibVirtエンドポイントの生成", func() {
		l, err = virt.NewLibVirtEp("qemu:///system")
		Expect(err).NotTo(HaveOccurred())
	})

	It("ドメインの定義", func() {
		err := l.DefineAndStartVM(*dom)
		Expect(err).NotTo(HaveOccurred())
	})

	It("ドメインのリスト取得", func() {
		nameList, err := virt.ListAllVm(l.Url)
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println("Active VM List:", nameList)
		Expect(len(nameList)).To(BeNumerically(">", 0))
		for _, name := range nameList {
			if name == hostname {
				GinkgoWriter.Println("Found VM:", name)
				return
			}
		}
	})

	//It("ドメインの削除", func() {
	//	err := l.DestroyDomain(hostname)
	//	Expect(err).NotTo(HaveOccurred())
	//	err = util.RemoveChannelDir(hostname)
	//	Expect(err).NotTo(HaveOccurred())
	//})
})

// 他テストスペック

// LVM

// インタフェース

// データボリューム
