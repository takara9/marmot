package virt_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/virt"
)

var _ = Describe("Virt", func() {

	Describe("Manipulation LibVirt", func() {

		Context("Lifecycle of LibVirt", func() {
			var in string = "test-data/vm-1.xml"
			var out string = "test-data/out-1.xml"
			var domain virt.Domain

			It("Read config XML file", func() {
				err := virt.ReadXml(in, &domain)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Domain = ", domain)
			})

			It("Set Domain Name temporaly", func() {
				virt.SetVmParam(&domain)
				GinkgoWriter.Println("Domain = ", domain)
			})

			It("Delete items in XML text file to avoid error", func() {
				xmlText := virt.CreateVirtXML(domain)
				GinkgoWriter.Println("xmlText = ", xmlText)
			})

			It("Write XML text file", func() {
				err := virt.WriteXml(out, &domain)
				Expect(err).NotTo(HaveOccurred())
			})

			It("List All virtual machines on same Hypervisor", func() {
				url := "qemu:///system"
				list, err := virt.ListAllVm(url)
				Expect(err).NotTo(HaveOccurred())
				for _, name := range list {
					GinkgoWriter.Println("VM Name = ", name)
				}
			})

			It("Create VM from XML text file", func() {
				url := "qemu:///system"
				fn := "test-data/vm-test.xml"

				err := virt.CreateStartVM(url, fn)
				Expect(err).NotTo(HaveOccurred())
			})

			It("List All virtual machines on same Hypervisor", func() {
				url := "qemu:///system"
				list, err := virt.ListAllVm(url)
				Expect(err).NotTo(HaveOccurred())
				for _, name := range list {
					GinkgoWriter.Println("VM Name = ", name)
				}
			})

			time.Sleep(time.Second * 30)

			It("Attache NIC the existing VM", func() {
				url := "qemu:///system"
				vm := "vm-test"
				fn := "test-data/nic-vlan1001.xml"
				err := virt.AttachDev(url, fn, vm)
				Expect(err).NotTo(HaveOccurred())
			})

			time.Sleep(time.Second * 30)

			It("Delete VM", func() {
				url := "qemu:///system"
				vm := "vm-test"
				err := virt.DestroyVM(url, vm)
				Expect(err).NotTo(HaveOccurred())
			})

			It("List All virtual machines on same Hypervisor", func() {
				url := "qemu:///system"
				list, err := virt.ListAllVm(url)
				Expect(err).NotTo(HaveOccurred())
				for _, name := range list {
					GinkgoWriter.Println("VM Name = ", name)
				}
			})
		})
	})
})
