package marmotd_test

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
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
	ut "github.com/takara9/marmot/pkg/util"
)

const (
	systemctl_exe = "/usr/bin/systemctl"
	hvadmin_exe   = "/usr/local/bin/hv-admin"
	etcdctl_exe   = "/usr/bin/etcdctl"
)

var ccf *string
var etcdUrl2 string = "http://127.0.0.1:5379"
var etcd *string = &etcdUrl2
var nodeName string = "hvc"
var node *string = &nodeName
var etcdEp *db.Database
var containerID2 string

func prepareMockVmfunc(){
	It("モックサーバーの起動", func(){
		e := echo.New()
		server := marmotd.NewServer("hvc", etcdUrl2)
		go func() {
			api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
			fmt.Println(e.Start("127.0.0.1:8080"), "Mock server is running")
		}()

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcdmarmot", "-p", "5379:2379", "-p", "5380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID2 = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID2)
		time.Sleep(10 * time.Second) // コンテナが起動するまで待機
	}, NodeTimeout(20*time.Second))
}

func cleanupMockVmfunc(){
	It("モックサーバーの終了", func(){
		// Dockerコンテナを停止・削除
		cmd := exec.Command("docker", "stop", containerID2)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerID2)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}
	}, NodeTimeout(20*time.Second))
}

func testMarmotFuncs() {
	Context("Data management", func() {
		It("Set up databae ", func() {
			var err error
			etcdEp, err = db.NewDatabase(etcdUrl2)
			Expect(err).NotTo(HaveOccurred())
		})

		var hvs config.Hypervisors_yaml
		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				err := etcdEp.SetHypervisors(hv)
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

		It("ストレージの空き容量チェック", func() {
			err := util.CheckHvVgAll(etcdUrl2, nodeName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Check up Marmot daemon", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8080/ping")
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
			GinkgoWriter.Println(out)
			Expect(err).To(Succeed()) // 成功
		})
	})

	Context("VMクラスタの生成と削除", func() {
		var cnf cf.MarmotConfig
		var m *marmotd.Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmotd.NewMarmot(*node, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Create Cluster()", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.CreateClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config for destroy", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.DestroyClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの生成と一時停止と再開", func() {
		var cnf cf.MarmotConfig
		var m *marmotd.Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmotd.NewMarmot(*node, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Create Cluster()", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.CreateClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Stop Cluster", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.StopClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Start Cluster", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.DestroyClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.DestroyClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの２重起動の防止", func() {
		var cnf cf.MarmotConfig
		var m *marmotd.Marmot

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmotd.NewMarmot(*node, *etcd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf = &fn
			err := cf.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスターの起動", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.CreateClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスターの２重起動 エラー発生が発生", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.CreateClusterInternal(newCnf)
			Expect(err).To(HaveOccurred())
		})

		It("Start Cluster", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.DestroyClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			newCnf := marmotd.ConvConfClusterOld2New(cnf)
			err := m.DestroyClusterInternal(newCnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
}
