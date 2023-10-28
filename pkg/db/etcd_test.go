package db

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	etcd "go.etcd.io/etcd/client/v3"
	//"errors"
)

var _ = Describe("Etcd", func() {

	const url = "http://127.0.0.1:2379"
	const url_noexist = "http://127.0.0.1:2380"
	//var testv string = 12
	var Conn *etcd.Client
	var err error

	Describe("Test etcd", func() {
		Context("Test Connection to etcd", func() {
			//It("Connection etcd not exist", func() {
			//	conn, err: = Connect(url_noexist)
			//	Expect(err).To(HaveOccurred())
		  	//})
			It("Connection etcd", func() {
				Conn, err = Connect(url)
				Expect(err).NotTo(HaveOccurred())
		  	})
		})

		Context("Test access etcd", func() {
			var key_hv1 = "hv01" 
			//var data_get Hypervisor
			data_hv1 := testHvData1()

			It("Put", func() {
				err := PutDataEtcd(Conn, key_hv1, data_hv1)
				Expect(err).NotTo(HaveOccurred())
		  	})

		  	It("Get", func() {
				data_get, err := GetHvByKey(Conn, key_hv1)
				Expect(err).NotTo(HaveOccurred())
				Expect(data_get.Cpu).To(Equal(data_hv1.Cpu))
		  	})

  	        It("Del", func() {
				err = DelByKey(Conn, key_hv1)	
				Expect(err).NotTo(HaveOccurred())
		  	})

		  	It("Check delete data", func() {
				_, err := GetHvByKey(Conn, key_hv1)
				Expect(err).To(HaveOccurred())
		  	})
		})

		Context("Test Sequence number", func() {
			It("Delete seqno key", func() {
				err := DelByKey(Conn, "test-serial")
				Expect(err).NotTo(HaveOccurred())
			})


			tests := []struct {
				name string
				want uint64
			}{
				{name: "Get Seq No inital", want: 1},
				{name: "Get Seq No 2nd", want: 2},
				{name: "Get Seq No 3rd", want: 3},
				{name: "Get Seq No 4th", want: 4},
			}

			for _, tt := range tests {
				It(tt.name, func(){
					seqno, err := GetSeq(Conn, "test-serial")
					Expect(err).To(HaveOccurred())
					Expect(seq).To(Equal(tt.want))
				})
			}

		})
	})
})
