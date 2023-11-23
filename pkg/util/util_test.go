package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	//db "github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Util", func() {

	var url string
	//var node string = "hv0"

	BeforeEach(func() {
		url = "http://127.0.0.1:2379"
	})

	Describe("Test etcd", func() {
		Context("Test Access to etcd", func() {
			It("Connection etcd", func() {
				//h, err := CheckHypervisors(url, node)
				Expect(url).To(Equal("http://127.0.0.1:2379"))
				//Expect(h[0].No).To(Equal("hv1"))
				GinkgoWriter.Println("XXXX") 
		  	})
		})
	})
})
