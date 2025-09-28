package marmot_test

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmot"
	"github.com/takara9/marmot/pkg/marmotd"
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
	//etcd_url := "http://127.0.0.1:12379"

	/*
		cmd := exec.Command(systemctl_exe, "stop", "etcd")
		stop := cmd.Run()
		Expect(stop).To(Succeed())

		cmd = exec.Command(systemctl_exe, "status", "etcd")
		status := cmd.Run()
		Expect(status).To(HaveOccurred())

		cmd = exec.Command(systemctl_exe, "start", "etcd")
		start := cmd.Run()
		Expect(start).To(Succeed())
	*/
	/*
		cmd := exec.Command(systemctl_exe, "stop", "marmot")
		stop := cmd.Run()
		Expect(stop).To(Succeed())

		cmd = exec.Command(systemctl_exe, "status", "marmot")
		status := cmd.Run()
		Expect(status).To(HaveOccurred())
	*/
})

// テスト後の環境戻し
var _ = AfterSuite(func() {
	// データの削除
	//cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:12379", "del", "hvc")
	//cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
	//out, err := cmd.CombinedOutput()
	//fmt.Println("command = ", string(out))
	//Expect(err).To(Succeed())

	// etcd の停止
	//cmd = exec.Command(systemctl_exe, "stop", "etcd")
	//status := cmd.Run()
	//Expect(status).To(Succeed())

	// marmotの停止
	/*
		cmd := exec.Command(systemctl_exe, "stop", "marmot")
		status := cmd.Run()
		Expect(status).To(Succeed())
	*/
})

var _ = Describe("Marmot", Ordered, func() {
	etcd_url := "http://127.0.0.1:5379"
	etcd = &etcd_url
	node_name := "127.0.0.1"
	node = &node_name
	var etcdEp *db.Database

	//var url string
	//var err error
	//var d *Database
	var containerID string

	BeforeAll(func(ctx SpecContext) {

		e := echo.New()
		server := marmotd.NewServer("hvc", "http://127.0.0.1:5379")
		go func() {
			api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
			fmt.Println(e.Start("0.0.0.0:8750"), "Mock server is running")
		}()

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcdmarmot", "-p", "5379:2379", "-p", "5380:2380", "-e", "ALLOW_NONE_AUTHENTICATION=yes", "-e", "ETCD_ADVERTISE_CLIENT_URLS=http://127.0.0.1:5379", "bitnami/etcd")
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

	Context("Data management", func() {
		//It("Set Hypervisor Config file", func() {
		/// ポート番号を変えられないとテストできない
		//	cmd := exec.Command(hvadmin_exe, "-config", "testdata/hypervisor-config-hvc.yaml")
		//	err := cmd.Run()
		//	Expect(err).NotTo(HaveOccurred())
		//})
		It("Set up databae ", func() {
			var err error
			etcdEp, err = db.NewDatabase(etcd_url)
			Expect(err).NotTo(HaveOccurred())
		})

		var hvs config.Hypervisors_yaml
		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				fmt.Println(hv)
				err := etcdEp.SetHypervisor(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := etcdEp.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のリセット", func() {
			for _, sq := range hvs.Seq {
				err := etcdEp.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		//It("ストレージの空き容量チェック", func() {
		//	err := util.CheckHvVgAll(marmotServer.Ma.EtcdUrl, marmotServer.Ma.NodeName)
		//	Expect(err).NotTo(HaveOccurred())
		//})

		//It("Start", func() {
		//	cmd := exec.Command(systemctl_exe, "start", "marmot")
		//	start := cmd.Run()
		//	Expect(start).To(Succeed()) // 成功
		//})

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
				GinkgoWriter.Println("xxxxxx hv port     == ", v.Port)
				GinkgoWriter.Println("xxxxxx hv CPU      == ", v.Cpu)
				GinkgoWriter.Println("xxxxxx hv Mem      == ", v.Memory)
				GinkgoWriter.Println("xxxxxx hv IP addr  == ", v.IpAddr)
			}
		})

		It("Check the config file to directly etcd", func() {
			cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:5379", "get", "hvc")
			cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
			out, err := cmd.CombinedOutput()
			fmt.Println("command = ", string(out))
			Expect(err).To(Succeed()) // 成功
		})
	})

	Context("VMクラスタの生成と削除", func() {
		var cnf cf.MarmotConfig
		var m *marmot.Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmot.NewMarmot(*node, *etcd)
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
			err := m.DestroyCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの生成と一時停止と再開", func() {
		var cnf cf.MarmotConfig
		var m *marmot.Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmot.NewMarmot(*node, *etcd)
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
			err := m.StopCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Start Cluster", func() {
			err := m.StartCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			err := m.DestroyCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの２重起動の防止", func() {
		var cnf cf.MarmotConfig
		var m *marmot.Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmot.NewMarmot(*node, *etcd)
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
			err := m.StartCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			err := m.DestroyCluster2(cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
