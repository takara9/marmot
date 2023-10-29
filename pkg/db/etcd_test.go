package db

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	etcd "go.etcd.io/etcd/client/v3"
	cf "github.com/takara9/marmot/pkg/config"
)

var _ = Describe("Etcd", func() {

	var url string
	//var url_noexist string
	var Conn *etcd.Client
	var err error

	BeforeEach(func() {
		url = "http://127.0.0.1:2379"
		//url_noexist = "http://127.0.0.1:2380"
		//book = &books.Book{
		//	Title: "Les Miserables",
		//	Author: "Victor Hugo",
		//	Pages: 2783,
		//	Weight: 500,
		//}
	})
	  
	AfterEach(func() {
		//err := os.Clearenv("WEIGHT_UNITS")
		//Expect(err).NotTo(HaveOccurred())
	})


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

			var IDX = "TST"

			It("Connection etcd", func() {
				Conn, err = Connect(url)
				Expect(err).NotTo(HaveOccurred())
		  	})

			It("Delete seqno key", func() {
				err := DelByKey(Conn, IDX)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Create seqno key", func() {
				err := CreateSeq(Conn, IDX, 1, 1)
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
				fmt.Println(i, td.name, td.want)
			
				It(td.name, func() {
					seqno, err := GetSeq(Conn, IDX)
					GinkgoWriter.Println("seqno ", seqno) 
					Expect(err).NotTo(HaveOccurred())
					Expect(seqno).To(Equal(uint64(td.want)))
				})
			}
			It("test loop", func() {
				for i, tt := range tests {
					fmt.Println(i, td.name, td.want)
					seqno, err := GetSeq(Conn, IDX)
					GinkgoWriter.Println("seqno ", seqno) 
					Expect(err).NotTo(HaveOccurred())
					Expect(seqno).To(Equal(uint64(tests[0].want)))
				}
			})
			*/

			It(tests[0].name, func() {
				seqno, err := GetSeq(Conn, IDX)
				GinkgoWriter.Println("seqno ", seqno) 
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[0].want)))
			})

			It(tests[1].name, func() {
				seqno, err := GetSeq(Conn, IDX)
				GinkgoWriter.Println("seqno ", seqno) 
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[1].want)))
			})

			It(tests[2].name, func() {
				seqno, err := GetSeq(Conn, IDX)
				GinkgoWriter.Println("seqno ", seqno) 

				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[2].want)))
			})

			It(tests[3].name, func() {
				seqno, err := GetSeq(Conn, IDX)
				GinkgoWriter.Println("seqno ", seqno) 

				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[3].want)))
			})

			It("Delete seqno key", func() {
				err := DelByKey(Conn, IDX)
				Expect(err).NotTo(HaveOccurred())
			})
		})



	})



	Describe("Read Hypervisor Config file and Check", func() {
		const hypervior_config string = "testdata/hypervisor-config.yaml"
		var cn cf.Hypervisors_yaml
		
		type hv struct {
			name string
			cpu  uint64
			ram  uint64
		}
		tests := []struct {
			name string
			want hv
		}{
			{name: "1st", want: hv{name: "hv1", cpu: 10, ram: 64}},
			{name: "2nd", want: hv{name: "hv2", cpu: 10, ram: 64}},
		}

		Context("Read a test hypervisor config file", func() {
			It("Read existing file", func() {
				err := cf.ReadConfig(hypervior_config, &cn)
				Expect(err).NotTo(HaveOccurred())
				for i, h := range cn.Hvs {
					GinkgoWriter.Println(i)
					GinkgoWriter.Println(h.Name)
					GinkgoWriter.Println(h.Cpu)
					Expect(h.Name).To(Equal(tests[i].want.name))
					Expect(h.Cpu).To(Equal(tests[i].want.cpu))
					Expect(h.Ram).To(Equal(tests[i].want.ram))
				}
		  	})
		})
	})




	Describe("HyperVisor Management Test", func() {
		const hypervior_config string = "testdata/hypervisor-config.yaml"
		var cnf cf.Hypervisors_yaml

		type hv struct {
			name string
			cpu  uint64
			ram  uint64
		}
		tests := []struct {
			name string
			want hv
		}{
			{name: "1st", want: hv{name: "hv1", cpu: 10, ram: 64}},
			{name: "2nd", want: hv{name: "hv2", cpu: 10, ram: 64}},
		}


		Context("Test Connection to etcd", func() {
			It("Connection etcd", func() {
				Conn, err = Connect(url)
				Expect(err).NotTo(HaveOccurred())
		  	})
		})


		// ハイパーバイザーのコンフィグを読んで、データベースを初期化
		Context("Test of Hypervisor management : Set up", func() {

			It("Read existing file", func() {
				err := cf.ReadConfig(hypervior_config, &cnf)
				Expect(err).NotTo(HaveOccurred())
				for i, h := range cnf.Hvs {
					GinkgoWriter.Println(i)
					GinkgoWriter.Println(h.Name)
					GinkgoWriter.Println(h.Cpu)
					Expect(h.Name).To(Equal(tests[i].want.name))
					Expect(h.Cpu).To(Equal(tests[i].want.cpu))
					Expect(h.Ram).To(Equal(tests[i].want.ram))
				}
		  	})

			It("PUT Hypervisor node data #2", func() {
				// ハイパーバイザー
				for _, hv := range cnf.Hvs {
					GinkgoWriter.Println(hv)
					SetHypervisor(Conn, hv)
				}
			
				// OSイメージテンプレート
				for _, hd := range cnf.Imgs {
					SetImageTemplate(Conn, hd)
				}
			
				// シーケンス番号のリセット
				for _, sq := range cnf.Seq {
					CreateSeq(Conn, sq.Key, sq.Start, sq.Step)
				}

			})


			It("Get Hypervisors status", func() {
				var hvs []Hypervisor
				err = GetHvsStatus(Conn, &hvs)
				Expect(err).NotTo(HaveOccurred())

				for _, h := range hvs {
					GinkgoWriter.Println("Nodename     ", h.Nodename) 
					GinkgoWriter.Println("  CPU        ", h.Cpu) 
					GinkgoWriter.Println("  Memory     ", h.Memory) 
					GinkgoWriter.Println("  FreeCpu    ", h.FreeCpu) 
					GinkgoWriter.Println("  FreeMemory ", h.FreeMemory) 
				}
			})
		})


		Context("Test of Hypervisor management : Schedule of virtual machines", func() {
			type vmReq struct {
				name string
				cpu  int
				ram  int
			}
			tests := []struct {
				name string
				req  vmReq
				want string
				cpu  int
				ram  int
			}{
				{name: "1st", req: vmReq{name: "node1", cpu: 4, ram: 8}, want: "hv1", cpu: 6, ram: 56},
				{name: "2nd", req: vmReq{name: "node2", cpu: 4, ram: 8}, want: "hv1", cpu: 2, ram: 48},
				{name: "3rd", req: vmReq{name: "node3", cpu: 4, ram: 8}, want: "hv2", cpu: 6, ram: 56},
				{name: "4th", req: vmReq{name: "node4", cpu: 4, ram: 8}, want: "hv2", cpu: 2, ram: 48},
				{name: "5th", req: vmReq{name: "node5", cpu: 4, ram: 8}, want: "", cpu: 0, ram: 0},
				{name: "6th", req: vmReq{name: "node6", cpu: 2, ram: 8}, want: "hv1", cpu: 0, ram: 40},
				{name: "7th", req: vmReq{name: "node7", cpu: 1, ram: 8}, want: "hv2", cpu: 1, ram: 40},
				{name: "8th", req: vmReq{name: "node8", cpu: 1, ram: 8}, want: "hv2", cpu: 0, ram: 32},
			}

			It("Get Hypervisors status", func() {
				var hvs []Hypervisor
				err = GetHvsStatus(Conn, &hvs)
				Expect(err).NotTo(HaveOccurred())

				for _, h := range hvs {
					GinkgoWriter.Println("Nodename     ", h.Nodename) 
					GinkgoWriter.Println("  CPU        ", h.Cpu) 
					GinkgoWriter.Println("  Memory     ", h.Memory) 
					GinkgoWriter.Println("  FreeCpu    ", h.FreeCpu) 
					GinkgoWriter.Println("  FreeMemory ", h.FreeMemory) 
					GinkgoWriter.Println("  Status     ", h.Status) 
				}
			})

			It("Scheduling a virtual machine to Hypervisor #1", func() {
				GinkgoWriter.Println("test-1 ") 
				td := tests[0]
				GinkgoWriter.Println("test-2 ") 
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				GinkgoWriter.Println("test-3 ") 
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				GinkgoWriter.Println("test-4 ") 
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #2", func() {
				td := tests[1]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #3", func() {
				td := tests[2]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #4", func() {
				td := tests[3]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #5", func() {
				td := tests[4]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).To(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #6", func() {
				td := tests[5]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #7", func() {
				td := tests[6]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})

			It("Scheduling a virtual machine to Hypervisor #8", func() {
				td := tests[7]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, err := AssignHvforVm(Conn, vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName) 
				GinkgoWriter.Println("key   ", key) 
				GinkgoWriter.Println("txid  ", txid) 
			})
		})
	})
})



			/*
			type hvReq struct {
				name string
				cpu  int
				ram  int
			}
		
			tests := []struct {
				name string
				req  hvReq
				want *int
			}{
				{name: "1st", req: hvReq{name: "hv1", cpu: 10, ram: 64}, want: nil},
				{name: "2nd", req: hvReq{name: "hv2", cpu: 10, ram: 64}, want: nil},
			}

			It("Delete existing data", func() {
				DelByKey(Conn, tests[0].req.name)
				DelByKey(Conn, tests[1].req.name)
			})
			
			It("PUT Hypervisor node data #1", func() {
				td := tests[0]
				GinkgoWriter.Println("testHvCreate ") 
				hv := testHvCreate(td.req.name, td.req.cpu, td.req.ram)
				GinkgoWriter.Println("PutDataEtcd ") 
				err := PutDataEtcd(Conn, td.req.name, hv)
				Expect(err).NotTo(HaveOccurred())
			})

			It("PUT Hypervisor node data #2", func() {
				td := tests[1]
				GinkgoWriter.Println("testHvCreate ") 
				hv := testHvCreate(td.req.name, td.req.cpu, td.req.ram)
				GinkgoWriter.Println("PutDataEtcd ") 
				err := PutDataEtcd(Conn, td.req.name, hv)
				Expect(err).NotTo(HaveOccurred())
			})
			*/

