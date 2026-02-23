// テストコードを作成することが第一優先となる
package virt_test

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"libvirt.org/go/libvirtxml"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("Networks", Ordered, func() {
	var l *virt.LibVirtEp
	var vnet []libvirtxml.Network
	BeforeAll(func(ctx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
		var err error
		l, err = virt.NewLibVirtEp("qemu:///system")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func(ctx SpecContext) {
		l.Close()

		// クリーンアップ: 作成したネットワークを削除
		nameList, err := l.ListNetworks()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println("Active Network List:", nameList)
		Expect(len(nameList)).To(BeNumerically(">", 0))
		for _, name := range nameList {
			GinkgoWriter.Println("Found Network:", name)
			if name == "default" || name == "host-bridge" || name == "ovs-network" {
				fmt.Println("Skipping deletion of default network:", name)
				continue
			} else {
				err := l.DeleteVirtualNetwork(name)
				Expect(err).NotTo(HaveOccurred())
			}
		}
	})

	Context("仮想ネットワークの作成", func() {
		It("仮想ネットワークの定義-20,21  IPなし", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-0"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("virbr-test0"),
				},
			}
			vnetxml, err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
			vnet = append(vnet, *vnetxml)
		})
		It("仮想ネットワークの定義-20,21", func() {
			GinkgoWriter.Println("Defining and starting virtual network:", vnet[0].Name)
			err := l.DefineAndStartVirtualNetwork(vnet[0])
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち-20,21", func() {
			GinkgoWriter.Println("Waiting for network to stabilize...")
			time.Sleep(1 * time.Second)
		})

		It("仮想ネットワークの削除-20,21", func() {
			GinkgoWriter.Println("Deleting virtual network:", vnet[0].Name)
			err := l.DeleteVirtualNetwork(vnet[0].Name)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-22,23  IPあり、NATなし", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-1"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("virbr-test1"),
					IpAddress:  util.StringPtr("192.168.200.1"),
					Netmask:    util.StringPtr("255.255.255.0"),
					Nat:        util.BoolPtr(false),
				},
			}
			vnetxml, err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
			vnet = append(vnet, *vnetxml)
		})
		It("仮想ネットワークの定義-22,23", func() {
			err := l.DefineAndStartVirtualNetwork(vnet[1])
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち-22,23", func() {
			GinkgoWriter.Println("Waiting for network to stabilize...")
			time.Sleep(1 * time.Second)
		})

		It("仮想ネットワークの削除-22,23", func() {
			GinkgoWriter.Println("Deleting virtual network:", vnet[1].Name)
			err := l.DeleteVirtualNetwork(vnet[1].Name)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-24,25  IPあり、NATなし", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-2"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName:       util.StringPtr("virbr-test2"),
					IpAddress:        util.StringPtr("192.168.200.2"),
					Netmask:          util.StringPtr("255.255.255.0"),
					Dhcp:             util.BoolPtr(true),
					DhcpStartAddress: util.StringPtr("192.168.200.3"),
					DhcpEndAddress:   util.StringPtr("192.168.200.254"),
				},
			}
			vnetxml, err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
			vnet = append(vnet, *vnetxml)
		})
		It("仮想ネットワークの定義-24,25", func() {
			err := l.DefineAndStartVirtualNetwork(vnet[2])
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち-24,25", func() {
			GinkgoWriter.Println("Waiting for network to stabilize...")
			time.Sleep(1 * time.Second)
		})

		It("仮想ネットワークの削除-24,25", func() {
			GinkgoWriter.Println("Deleting virtual network:", vnet[2].Name)
			err := l.DeleteVirtualNetwork(vnet[2].Name)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-26,27  IPあり、NATあり", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-3"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName:       util.StringPtr("virbr-test3"),
					IpAddress:        util.StringPtr("192.168.200.1"),
					Netmask:          util.StringPtr("255.255.255.0"),
					Dhcp:             util.BoolPtr(true),
					DhcpStartAddress: util.StringPtr("192.168.200.2"),
					DhcpEndAddress:   util.StringPtr("192.168.200.254"),
					Nat:              util.BoolPtr(true),
				},
			}
			vnetxml, err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
			vnet = append(vnet, *vnetxml)
		})
		It("仮想ネットワークの定義-26,27", func() {
			err := l.DefineAndStartVirtualNetwork(vnet[3])
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち-26,27", func() {
			GinkgoWriter.Println("Waiting for network to stabilize...")
			time.Sleep(1 * time.Second)
		})

		It("仮想ネットワークの削除-26,27", func() {
			GinkgoWriter.Println("Deleting virtual network:", vnet[3].Name)
			err := l.DeleteVirtualNetwork(vnet[3].Name)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークのリスト取得", func() {
			Eventually(func(g Gomega) {
				nameList, err := l.ListNetworks()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Active Network List:", nameList)
				Expect(len(nameList)).To(BeNumerically(">", 0))
				for _, name := range nameList {
					GinkgoWriter.Println("Found Network:", name)
				}
			}).Should(Succeed())
		})
	})
})
