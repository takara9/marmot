package marmotd_test

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	//systemctl_exe = "/usr/bin/systemctl"
	//hvadmin_exe   = "/usr/local/bin/hv-admin"
	etcdctl_exe = "/usr/bin/etcdctl"
)

var etcdUrlTest string = "http://127.0.0.1:5379"
var etcdTest *string = &etcdUrlTest
var nodeName string = "hvc"
var nodeNamePtr *string = &nodeName
var etcdEpTest *db.Database
var etcdContainerIdFunc string

func prepareMockVmfunc() {
	fmt.Println("モックサーバーの起動")

	e := echo.New()
	server := marmotd.NewServer("hvc", etcdUrlTest)
	go func() {
		// Setup slog
		opts := &slog.HandlerOptions{
			AddSource: true,
			//Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("127.0.0.1:8090"), "Mock server is running")
	}()

	// Dockerコンテナを起動
	cmd := exec.Command("docker", "run", "-d", "--name", "etcdmarmot", "-p", "5379:2379", "-p", "5380:2380", "ghcr.io/takara9/etcd:3.6.5")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred())
	etcdContainerIdFunc = string(output[:12]) // 最初の12文字をIDとして取得
	fmt.Printf("Container started with ID: %s\n", etcdContainerIdFunc)
	time.Sleep(10 * time.Second) // コンテナが起動するまで待機
}

func cleanupMockVmfunc() {
	fmt.Println("モックサーバーの終了")
	// Dockerコンテナを停止・削除
	cmd := exec.Command("docker", "stop", etcdContainerIdFunc)
	_, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to stop container: %v\n", err)
	}
	cmd = exec.Command("docker", "rm", etcdContainerIdFunc)
	_, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to remove container: %v\n", err)
	}
}

func testMarmotFuncs() {
	Context("Data management", func() {
		It("Set up databae ", func() {
			var err error
			etcdEpTest, err = db.NewDatabase(etcdUrlTest)
			Expect(err).NotTo(HaveOccurred())
		})

		var hvs config.Hypervisors_yaml
		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc-func.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				err := etcdEpTest.SetHypervisors(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := etcdEpTest.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のセット", func() {
			for _, sq := range hvs.Seq {
				err := etcdEpTest.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Check up Marmot daemon", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8090/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("Check Hypervisors data", func() {
			GinkgoWriter.Println(*nodeNamePtr)
			hv, err := etcdEpTest.CheckHypervisors(*etcdTest, *nodeNamePtr)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("xxxxxx array size == ", len(hv))
			for i, v := range hv {
				GinkgoWriter.Println("xxxxxx hv index    == ", i)
				GinkgoWriter.Println("xxxxxx hv nodename == ", v.NodeName)
				GinkgoWriter.Println("xxxxxx hv port     == ", *v.Port)
				GinkgoWriter.Println("xxxxxx hv CPU      == ", v.Cpu)
				GinkgoWriter.Println("xxxxxx hv Mem      == ", *v.Memory)
				GinkgoWriter.Println("xxxxxx hv IP addr  == ", *v.IpAddr)
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
		var m *marmotd.Marmot
		var cnf *api.MarmotConfig
		var err error

		It("Create Marmot Instance", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config for Create Cluster", func() {
			By("Loading cluster config")
			cnf, err = config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
			Expect(err).NotTo(HaveOccurred())
			//marmotd.PrintMarmotConfig(*cnf)
		})

		It("Create Cluster", func() {
			err = m.CreateClusterInternal(*cnf) // VMがDBに登録されていない？
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster", func() {
			err = m.DestroyClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの生成と一時停止と再開", func() {
		var m *marmotd.Marmot
		var cnf *api.MarmotConfig
		var err error

		It("Create Marmot Instance", func() {
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Load Config for Create cluster", func() {
			cnf, err = config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
			Expect(err).NotTo(HaveOccurred())
		})

		It("Create cluster", func() {
			err = m.CreateClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Stop Cluster", func() {
			err = m.StopClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Start Cluster", func() {
			err = m.DestroyClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			By("Destroying cluster")
			err = m.DestroyClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("VMクラスタの２重起動の防止", func() {
		var m *marmotd.Marmot
		var cnf *api.MarmotConfig
		var err error

		It("Create Marmot Instance", func() {
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスターコンフィグの読み取り", func() {
			cnf, err = config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスターの起動", func() {
			err = m.CreateClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスターの２重起動 エラー発生が発生", func() {
			err = m.CreateClusterInternal(*cnf)
			Expect(err).To(HaveOccurred())
		})

		It("Start Cluster", func() {
			err = m.DestroyClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Destroy Cluster()", func() {
			err = m.DestroyClusterInternal(*cnf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
}
