package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Util", func() {

	var url string
	BeforeEach(func() {
		url = "http://127.0.0.1:2379"
	})

	Describe("Test etcd", func() {
		Context("Test Access to etcd", func() {
			It("Connection etcd", func() {
				Expect(url).To(Equal("http://127.0.0.1:2379"))
				GinkgoWriter.Println("XXXX") 
		  	})
		})
	})
})
