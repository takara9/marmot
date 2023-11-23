package lvm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
	"fmt"
)

var _ = Describe("Lvm", func() {


	Describe("Manipulation Logical Volume", func() {

		Context("Lifecycle of Logical Volume", func() {

			var vg = "vg1"
			var lv = "test-lv01"
			var sz uint64 = 1024 * 1024 * 1024 * 4  // GB
		
			It("Create Logical Volume", func() {
				err := CreateLV(vg, lv, sz)
				Expect(err).NotTo(HaveOccurred())
		  	})

		  	time.Sleep(time.Second * 30)

		  	It("Existing Check", func() {
				err := IsExist(vg, lv)
				Expect(err).NotTo(HaveOccurred())
			})

			time.Sleep(time.Second * 30)

			It("Remove Logical Volume", func() {
				err := RemoveLV(vg, lv)
				Expect(err).NotTo(HaveOccurred())
		  	})

		  	It("Existing Check", func() {
				err := IsExist(vg, lv)
				Expect(err).To(HaveOccurred())
		  	})
		})

		var vg = "vg1"
		var lv_template = "lv01"
		var lv_snapshot = "test-lv21"
		var sz_snapshot uint64 = 1024 * 1024 * 1024 * 1  // GB

		Context("Create Snapshot from Logical Volume", func() {
			It("Create Snapshot Volume", func() {
				Expect(CreateSnapshot(vg, lv_template, lv_snapshot, sz_snapshot)).NotTo(HaveOccurred())
		  	})
		  	It("Remove Snapshot Volume", func() {
				err := RemoveLV(vg, lv_snapshot)
				Expect(err).NotTo(HaveOccurred())
		  	})
		})

		Context("Volume Group", func() {
			It("Get Volume Group Info", func() {
				size,free, err := CheckVG(vg)
				Expect(err).NotTo(HaveOccurred())
				sizeg := size/1024/1024/1024
				freeg := free/1024/1024/1024
				fmt.Println("size = ",sizeg)
				fmt.Println("free = ",freeg)
		  })
		})
	})
})
