package virt

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
	//"virt"
)

var _ = Describe("Virt", func() {

	Describe("Manipulation LibVirt", func() {

		Context("Lifecycle of LibVirt", func() {
			var in string = "test-data/vm-1.xml"
			var out string = "test-data/out-1.xml"
			var domain Domain
		
			It("Read config XML file", func() {
				err := ReadXml(in, &domain)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Domain = ", domain) 
		  	})

			It("Set Domain Name temporaly", func() {
				SetVmParam(&domain)
				GinkgoWriter.Println("Domain = ", domain) 
		  	})

            It("Delete items in XML text file to avoid error", func() {
				xmlText := CreateVirtXML(domain)
				GinkgoWriter.Println("xmlText = ", xmlText) 
		  	})

            It("Write XML text file", func() {
				err := WriteXml(out, &domain)
				Expect(err).NotTo(HaveOccurred())
		  	})

			 It("List All virtual machines on same Hypervisor", func() {
				url := "qemu:///system"
				list, err := ListAllVm(url)
				Expect(err).NotTo(HaveOccurred())
				for _, name := range list {
					GinkgoWriter.Println("VM Name = ", name) 
				}
		  	})
		
			It("Create VM from XML text file", func() {
				url := "qemu:///system"
				fn := "test-data/vm-test.xml"
			
				err := CreateStartVM(url, fn)
				Expect(err).NotTo(HaveOccurred())
		  	})
			
            It("List All virtual machines on same Hypervisor", func() {
				url := "qemu:///system"
				list, err := ListAllVm(url)
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
				err := AttachDev(url, fn, vm)
				Expect(err).NotTo(HaveOccurred())
		  	})

		    time.Sleep(time.Second * 30)

			It("Attache NIC the existing VM", func() {
				url := "qemu:///system"
				vm := "vm-test"
				err := DestroyVM(url, vm)
				Expect(err).NotTo(HaveOccurred())
			})

			It("List All virtual machines on same Hypervisor", func() {
				url := "qemu:///system"
				list, err := ListAllVm(url)
				Expect(err).NotTo(HaveOccurred())
				for _, name := range list {
					GinkgoWriter.Println("VM Name = ", name) 
				}
		  	})
		})
	})
})
