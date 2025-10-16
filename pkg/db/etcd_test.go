package db

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cf "github.com/takara9/marmot/pkg/config"
	. "github.com/takara9/marmot/pkg/types"
)

var _ = Describe("Etcd", Ordered, func() {
	var url string
	var err error
	var d *Database
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		url = "http://127.0.0.1:4379"
		cmd := exec.Command("docker", "run", "-d", "--name", "etcddb", "-p", "4379:2379", "-p", "4380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)
		time.Sleep(10 * time.Second) // コンテナが起動するまで待機
	}, NodeTimeout(20*time.Second))

	AfterAll(func(ctx SpecContext) {
		// Dockerコンテナを停止・削除
		cmd := exec.Command("docker", "stop", containerID)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerID)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}
	}, NodeTimeout(20*time.Second))

	BeforeEach(func() {
	})

	AfterEach(func() {
	})

	Describe("Test etcd", func() {
		Context("Test Connection to etcd", func() {
			It("Connection etcd", func() {
				d, err = NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Test access etcd", func() {
			var key_hv1 = "hv01"
			data_hv1 := testHvData1()

			It("Put", func() {
				err := d.PutDataEtcd(key_hv1, data_hv1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Get", func() {
				data_get, err := d.GetHypervisorByKey(key_hv1)
				Expect(err).NotTo(HaveOccurred())
				Expect(data_get.Cpu).To(Equal(data_hv1.Cpu))
			})

			It("Del", func() {
				err = d.DelByKey(key_hv1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Check delete data", func() {
				_, err := d.GetHypervisorByKey(key_hv1)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("Test Sequence number", func() {
			var IDX = "TST"

			It("Delete seqno key", func() {
				err := d.DelByKey(IDX)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Create seqno key", func() {
				err := d.CreateSeq(IDX, 1, 1)
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

			It(tests[0].name, func() {
				seqno, err := d.GetSeq(IDX)
				GinkgoWriter.Println("seqno ", seqno)
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[0].want)))
			})

			It(tests[1].name, func() {
				seqno, err := d.GetSeq(IDX)
				GinkgoWriter.Println("seqno ", seqno)
				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[1].want)))
			})

			It(tests[2].name, func() {
				seqno, err := d.GetSeq(IDX)
				GinkgoWriter.Println("seqno ", seqno)

				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[2].want)))
			})

			It(tests[3].name, func() {
				seqno, err := d.GetSeq(IDX)
				GinkgoWriter.Println("seqno ", seqno)

				Expect(err).NotTo(HaveOccurred())
				Expect(seqno).To(Equal(uint64(tests[3].want)))
			})

			It("Delete seqno key", func() {
				err := d.DelByKey(IDX)
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
					d.SetHypervisors(hv)
				}

				// OSイメージテンプレート
				for _, hd := range cnf.Imgs {
					d.SetImageTemplate(hd)
				}

				// シーケンス番号のリセット
				for _, sq := range cnf.Seq {
					d.CreateSeq(sq.Key, sq.Start, sq.Step)
				}
			})

			It("Get Hypervisors status", func() {
				var hvs []Hypervisor
				err = d.GetHypervisors(&hvs)
				Expect(err).NotTo(HaveOccurred())
				for _, h := range hvs {
					GinkgoWriter.Println("Nodename     ", h.Nodename)
					GinkgoWriter.Println("  PORT       ", h.Port)
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
				err = d.GetHypervisors(&hvs)
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
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				GinkgoWriter.Println("test-4 ")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #2", func() {
				td := tests[1]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #3", func() {
				td := tests[2]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #4", func() {
				td := tests[3]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #5", func() {
				td := tests[4]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).To(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #6", func() {
				td := tests[5]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #7", func() {
				td := tests[6]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})

			It("Scheduling a virtual machine to Hypervisor #8", func() {
				td := tests[7]
				vm := testVmCreate(td.req.name, td.req.cpu, td.req.ram)
				hvName, key, txid, port, err := d.AssignHvforVm(vm)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("hvNam ", hvName)
				GinkgoWriter.Println("port  ", port)
				GinkgoWriter.Println("key   ", key)
				GinkgoWriter.Println("txid  ", txid)
			})
		})
	})
})
