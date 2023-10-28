package db

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"db"
	"errors"
)

var _ = Describe("Etcd", func() {

	const url = "http://127.0.0.1:2379"
	const url_noexist = "http://127.0.0.1:2380"
	var testv = 12
	var Conn *etcd.Client
	var err errors

	Describe("Test etcd", func() {
		Context("Test Connection to etcd", func() {
			It("Connection etcd not exist", func() {
				Conn, err = Connect(url_noexist)
				Expect(err).To(HaveOccurred())
		  	})
			It("Connection etcd", func() {
				Conn, err = Connect(url)
				Expect(err).NotTo(HaveOccurred())
		  	})
		})

		Context("Test access etcd", func() {
			var key_data = "hv01" 
			var data_hv1 Hypervisor

			It("Put", func() {
				data_hv1 := testHvData1()
				err := PutDataEtcd(Conn, key_data, data_hv1)
				Expect(err).NotTo(HaveOccurred())
		  	})

		  	It("Get", func() {
				data_get, err := GetHvByKey(Conn, key_hv1)
				Expect(err).NotTo(HaveOccurred())
				Expect(data_get.Cpu).To(Equal(data_hv1.Cpu))
		  	})

  	        It("Del", func() {
				err = DelByKey(Conn, key)	
				Expect(err).NotTo(HaveOccurred())
		  	})

		  	It("Check delete data", func() {
				data_get, err := GetHvByKey(Conn, key_hv1)
				Expect(err).To(HaveOccurred())
		  	})
		})

		Context("Test Sequence number", func() {
			It("Delete seqno key", func() {
				err := DelByKey(Conn, "test-serial")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
