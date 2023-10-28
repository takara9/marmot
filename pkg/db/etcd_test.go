package db

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	etcd "go.etcd.io/etcd/client/v3"
	//"fmt"
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

			It("Connection etcd", func() {
				Conn, err = Connect(url)
				Expect(err).NotTo(HaveOccurred())
		  	})

			It("Delete seqno key", func() {
				err := DelByKey(Conn, "TST")
				Expect(err).NotTo(HaveOccurred())
			})

			It("Create seqno key", func() {
				err := CreateSeq(Conn, "TST", 1, 1)
				Expect(err).NotTo(HaveOccurred())
			})


			tests := []struct {
				name string
				want uint64
			}{
				{name: "Get Seq No inital", want: 1},
				{name: "Get Seq No 2nd",    want: 2},
				{name: "Get Seq No 3rd",    want: 3},
				{name: "Get Seq No 4th",    want: 4},
			}

			/*
			ループを入れることは許されないみたい
			for i, tt := range tests {
				fmt.Println(i, tt.name, tt.want)
			
				It(tt.name, func() {
					seqno, err := GetSeq(Conn, "TST")
					GinkgoWriter.Println("seqno ", seqno) 
					Expect(err).NotTo(HaveOccurred())
					Expect(seqno).To(Equal(uint64(tt.want)))
				})
			}
			It("test loop", func() {
				for i, tt := range tests {
					fmt.Println(i, tt.name, tt.want)
					seqno, err := GetSeq(Conn, "TST")
					GinkgoWriter.Println("seqno ", seqno) 
					Expect(err).NotTo(HaveOccurred())
					Expect(seqno).To(Equal(uint64(tests[0].want)))
				}
			})
			*/

			It(tests[0].name, func() {
				seqno, err := GetSeq(Conn, "TST")
				GinkgoWriter.Println("seqno ", seqno) 
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[0].want)))
			})

			It(tests[1].name, func() {
				seqno, err := GetSeq(Conn, "TST")
				GinkgoWriter.Println("seqno ", seqno) 
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[1].want)))
			})

			It(tests[2].name, func() {
				seqno, err := GetSeq(Conn, "TST")
				GinkgoWriter.Println("seqno ", seqno) 
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[2].want)))
			})

			It(tests[3].name, func() {
				seqno, err := GetSeq(Conn, "TST")
				GinkgoWriter.Println("seqno ", seqno) 
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[3].want)))
			})

		})
	})
})
