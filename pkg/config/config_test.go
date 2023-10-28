package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {

	const input1   string = "testdata/ceph-cluster.yaml"
	const input2   string = "testdata/no-exist.yaml"
	const output1  string = "testdata/ceph-cluster-out.yaml"
	var mc MarmotConfig
	
	Describe("Read / Write config file", func() {
		Context("Read a test config file", func() {
			It("Read existing file", func() {
				err := ReadConfig(input1, &mc)
				Expect(err).NotTo(HaveOccurred())
				Expect(mc.Domain).To(Equal("labo.local"))
				Expect(mc.VMSpec[0].Name).To(Equal("node1"))
		  	})
			  It("Read no existing file", func() {
				err := ReadConfig(input2, &mc)
				Expect(err).To(HaveOccurred())
		  	})
		})
		Context("Write a test config file", func() {
			It("Write file", func() {
				err := WriteConfig(output1, mc)
				Expect(err).NotTo(HaveOccurred())
		  	})
			  It("Write file, but can not", func() {
				err := WriteConfig("testdata", mc)
				Expect(err).To(HaveOccurred())
		  	})
		})
	})
})
