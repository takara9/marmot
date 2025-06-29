package util_test

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
	"os/exec"

	cf "github.com/takara9/marmot/pkg/config"
	ut "github.com/takara9/marmot/pkg/util"
)

const (
	systemctl_exe = "/usr/bin/systemctl"
	hvadmin_exe   = "/usr/local/bin/hv-admin"
	etcdctl_exe   = "/usr/bin/etcdctl"
)

var node *string
var etcd *string
var ccf *string

//var cnf cf.MarmotConfig

//var etcd_url string

var hv_config string

var _ = BeforeSuite(func() {
	etcd_url := "http://127.0.0.1:12379"
	etcd = &etcd_url
	node_name := "127.0.0.1"
	node = &node_name
	fmt.Println("node = ", *node)
	fmt.Println("etcd = ", *etcd)
	config_fn := "testdata/cluster-config.yaml"
	ccf = &config_fn
	fmt.Println("ccf  = ", *ccf)
	hv_config = "testdata/hypervisor-config-hvc.yaml"
})

var _ = AfterSuite(func() {
	// データの削除
	{
		cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:12379", "del", "hvc")
		cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
		out, err := cmd.CombinedOutput()
		fmt.Println("command = ", string(out))
		Expect(err).To(Succeed()) // 成功
	}

	// etcd の停止
	{
		cmd := exec.Command(systemctl_exe, "stop", "etcd")
		_ = cmd.Run()
	}

	// corednsの停止
	{
		cmd := exec.Command(systemctl_exe, "stop", "coredns")
		_ = cmd.Run()
	}

	// marmotの停止
	{
		cmd := exec.Command(systemctl_exe, "stop", "marmot")
		_ = cmd.Run()
	}
})

var _ = Describe("Util", func() {

	BeforeEach(func() {
		/* HV設定ファイルをロードするコードを適正化して必要なコードを組み込む */
		fmt.Println("node = ", *node)
		fmt.Println("etcd = ", *etcd)
		fmt.Println("ccf  = ", *ccf)
		fmt.Println("hv_config  = ", hv_config)
	})

	Describe("Check daemons", func() {
		Context("etcd", func() {
			It("Stop", func() {
				cmd := exec.Command(systemctl_exe, "stop", "etcd")
				stop := cmd.Run()
				Expect(stop).To(Succeed()) // 成功
			})

			It("Status", func() {
				cmd := exec.Command(systemctl_exe, "status", "etcd")
				status := cmd.Run()
				Expect(status).To(HaveOccurred()) // 問題
			})

			It("Start", func() {
				cmd := exec.Command(systemctl_exe, "start", "etcd")
				start := cmd.Run()
				Expect(start).To(Succeed()) // 成功
			})
		})

		Context("marmot", func() {
			It("Stop", func() {
				cmd := exec.Command(systemctl_exe, "stop", "marmot")
				stop := cmd.Run()
				Expect(stop).To(Succeed()) // 成功
			})

			It("Status", func() {
				cmd := exec.Command(systemctl_exe, "status", "marmot")
				status := cmd.Run()
				Expect(status).To(HaveOccurred()) // 問題
			})

			It("Start", func() {
				cmd := exec.Command(systemctl_exe, "start", "marmot")
				start := cmd.Run()
				Expect(start).To(Succeed()) // 成功
			})
		})
	})

	Context("Data management", func() {
		It("Check up Marmot daemon", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8750/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("Set Hypervisor Config file", func() {
			cmd := exec.Command(hvadmin_exe, "-config", hv_config)
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("Check Hypervisors data", func() {
			GinkgoWriter.Println(*node)
			hv, err := ut.CheckHypervisors(*etcd, *node)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("xxxxxx array size == ", len(hv))
			for i, v := range hv {
				GinkgoWriter.Println("xxxxxx hv index    == ", i)
				GinkgoWriter.Println("xxxxxx hv nodename == ", v.Nodename)
				GinkgoWriter.Println("xxxxxx hv CPU      == ", v.Cpu)
				GinkgoWriter.Println("xxxxxx hv Mem      == ", v.Memory)
				GinkgoWriter.Println("xxxxxx hv IP addr  == ", v.IpAddr)
			}
		})

		It("Check the config file to directly etcd", func() {
			cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:12379", "get", "hvc")
			cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
			out, err := cmd.CombinedOutput()
			fmt.Println("command = ", string(out))
			Expect(err).To(Succeed()) // 成功
		})
	})

	Context("func test", func() {
		var cnf cf.MarmotConfig
		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Create Cluster()", func() {
			err := ut.CreateCluster(cnf, *etcd, *node)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config for destroy", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Destroy Cluster()", func() {
			err := ut.DestroyCluster(cnf, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("func test2", func() {
		var cnf cf.MarmotConfig

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Create Cluster()", func() {
			err := ut.CreateCluster(cnf, *etcd, *node)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Stop Cluster", func() {
			err := ut.StopCluster(cnf, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Start Cluster", func() {
			err := ut.StartCluster(cnf, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		//It("Load Config for destroy", func() {
		//	fn := "testdata/cluster-config.yaml"
		//	ccf = &fn
		//	err := cf.ReadConfig(*ccf, &cnf)
		//	Expect(err).NotTo(HaveOccurred())
		//})

		It("Destroy Cluster()", func() {
			err := ut.DestroyCluster(cnf, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
