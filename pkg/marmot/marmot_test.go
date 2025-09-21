package marmot

import (
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cf "github.com/takara9/marmot/pkg/config"
	ut "github.com/takara9/marmot/pkg/util"
)

const (
	systemctl_exe = "/usr/bin/systemctl"
	hvadmin_exe   = "/usr/local/bin/hv-admin"
	etcdctl_exe   = "/usr/bin/etcdctl"
)

var ccf *string
var etcd *string
var node *string

// テスト前の環境設定
var _ = BeforeSuite(func() {
	etcd_url := "http://127.0.0.1:12379"
	etcd = &etcd_url
	node_name := "127.0.0.1"
	node = &node_name

	cmd := exec.Command(systemctl_exe, "stop", "etcd")
	stop := cmd.Run()
	Expect(stop).To(Succeed())

	cmd = exec.Command(systemctl_exe, "status", "etcd")
	status := cmd.Run()
	Expect(status).To(HaveOccurred())

	cmd = exec.Command(systemctl_exe, "start", "etcd")
	start := cmd.Run()
	Expect(start).To(Succeed())

	cmd = exec.Command(systemctl_exe, "stop", "marmot")
	stop = cmd.Run()
	Expect(stop).To(Succeed())

	cmd = exec.Command(systemctl_exe, "status", "marmot")
	status = cmd.Run()
	Expect(status).To(HaveOccurred())
})

// テスト後の環境戻し
var _ = AfterSuite(func() {
	// データの削除
	cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:12379", "del", "hvc")
	cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
	out, err := cmd.CombinedOutput()
	fmt.Println("command = ", string(out))
	Expect(err).To(Succeed())

	// etcd の停止
	cmd = exec.Command(systemctl_exe, "stop", "etcd")
	status := cmd.Run()
	Expect(status).To(Succeed())

	// marmotの停止
	cmd = exec.Command(systemctl_exe, "stop", "marmot")
	status = cmd.Run()
	Expect(status).To(Succeed())

})

var _ = Describe("Marmot", func() {
	Context("Data management", func() {
		It("Set Hypervisor Config file", func() {
			cmd := exec.Command(hvadmin_exe, "-config", "testdata/hypervisor-config-hvc.yaml")
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("Start", func() {
			cmd := exec.Command(systemctl_exe, "start", "marmot")
			start := cmd.Run()
			Expect(start).To(Succeed()) // 成功
		})

		It("Check up Marmot daemon", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8750/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
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

	Context("VMクラスタの生成と削除", func() {
		var cnf cf.MarmotConfig
		var m *Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = NewMarmot(*node, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Create Cluster()", func() {
			err := m.CreateCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config for destroy", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			err := m.destroyCluster(cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの生成と一時停止と再開", func() {
		var cnf cf.MarmotConfig
		var m *Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = NewMarmot(*node, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Create Cluster()", func() {
			err := m.CreateCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Stop Cluster", func() {
			err := m.stopCluster(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Start Cluster", func() {
			err := m.startCluster(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			err := m.destroyCluster(cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの２重起動の防止", func() {
		var cnf cf.MarmotConfig
		var m *Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = NewMarmot(*node, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})
		It("クラスターの起動", func() {
			err := m.CreateCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスターの２重起動 エラー発生が発生", func() {
			err := m.CreateCluster2(cnf)
			Expect(err).To(HaveOccurred())
		})

		It("Start Cluster", func() {
			err := m.startCluster(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			err := m.destroyCluster(cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
