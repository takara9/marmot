// テストコードを作成することが第一優先となる
package virt_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("Networks", func() {
	//var net *api.VirtualNetwork

	Context("仮想ネットワークの作成1", func() {
		// テストケースの検証ができていない

		It("仮想ネットワークの定義-1  IPTなし", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-0"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("virbr-test0"),
				},
			}
			err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).To(HaveOccurred())
		})

		It("仮想ネットワークの定義-1  NATなし", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-1"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("virbr-test1"),
					IpAddress:  util.StringPtr("192.168.122.1"),
					Netmask:    util.StringPtr("255.255.255.0"),
					Nat:        util.BoolPtr(false),
				},
			}
			err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-2  NATあり", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-2"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("virbr-test2"),
					IpAddress:  util.StringPtr("192.168.122.1"),
					Netmask:    util.StringPtr("255.255.255.0"),
					Nat:        util.BoolPtr(true),
				},
			}
			err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-2  DHCPなし", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-2"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("virbr-test2"),
					IpAddress:  util.StringPtr("192.168.122.1"),
					Netmask:    util.StringPtr("255.255.255.0"),
					Dhcp:       util.BoolPtr(false),
				},
			}
			err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-2  DHCPあり", func() {
			net := &api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name: util.StringPtr("test-net-2"),
					Uuid: util.StringPtr(uuid.New().String()),
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName:       util.StringPtr("virbr-test2"),
					IpAddress:        util.StringPtr("192.168.122.1"),
					Netmask:          util.StringPtr("255.255.255.0"),
					Dhcp:             util.BoolPtr(true),
					DhcpStartAddress: util.StringPtr("192.168.122.2"),
					DhcpEndAddress:   util.StringPtr("192.168.122.254"),
				},
			}
			err := virt.CreateVirtualNetworkXML(*net)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	/*
		It("LibVirtエンドポイントの生成", func() {
			l, err = virt.NewLibVirtEp("qemu:///system")
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの定義-1", func() {
			err := l.DefineAndStartVM(*dom1)
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち", func() {
			time.Sleep(3 * time.Second)
		})

		It("仮想ネットワークのリスト取得", func() {
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

		It("仮想ネットワークの削除", func() {
			err := l.DeleteDomain(hostname1)
			Expect(err).NotTo(HaveOccurred())
			err = util.RemoveChannelDir(hostname1)
			Expect(err).NotTo(HaveOccurred())
		})
	*/

})
