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
	ut "github.com/takara9/marmot/pkg/util"
)

var etcdContainerIdVol string

func prepareMockVolume() {
	fmt.Println("モックサーバーの起動 for ボリュームテスト")

	e := echo.New()
	server := marmotd.NewServer("hvc", etcdUrlTest)
	go func() {
		// Setup slog
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("127.0.0.1:8092"), "Mock server is running")
	}()

	// Dockerコンテナを起動
	cmd := exec.Command("docker", "run", "-d", "--name", "etcdvolume", "-p", "7379:2379", "-p", "7380:2380", "ghcr.io/takara9/etcd:3.6.5")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred())
	etcdContainerIdVol = string(output[:12]) // 最初の12文字をIDとして取得
	fmt.Printf("Container started with ID: %s\n", etcdContainerIdVol)
	time.Sleep(10 * time.Second) // コンテナが起動するまで待機
}

func cleanupMockVolume() {
	fmt.Println("ボリュームテスト用モックサーバーの終了")
	// Dockerコンテナを停止・削除
	cmd := exec.Command("docker", "stop", etcdContainerIdVol)
	_, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to stop container: %v\n", err)
	}
	cmd = exec.Command("docker", "rm", etcdContainerIdVol)
	_, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to remove container: %v\n", err)
	}
}

func testMarmotVolumes() {
	Context("テストデータの初期化", func() {
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
				cmd := exec.Command("curl", "http://localhost:8092/ping")
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
			cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:7379", "get", "hvc")
			cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
			out, err := cmd.CombinedOutput()
			GinkgoWriter.Println(out)
			Expect(err).To(Succeed()) // 成功
		})
	})

	Context("OSボリュームの生成から削除", func() {
		var m *marmotd.Marmot
		var volKey string
		var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("OSボリュームの生成", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("lvm"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volKey, err = m.CreateVolume(v)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", volKey)
		})

		It("OSボリュームの削除", func() {
			err = m.RemoveVolume(volKey)
			Expect(err).NotTo(HaveOccurred())
		})

		It("OSボリュームの生成 （失敗ケース)", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("lvm"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.NOXIST"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volKey, err = m.CreateVolume(v)
			Expect(err).To(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", volKey)
		})

		It("OSボリュームの生成 （失敗ケース)", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("noexist"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volKey, err = m.CreateVolume(v)
			GinkgoWriter.Println("Created volume key: ", volKey)
			GinkgoWriter.Println("err=", err)
			Expect(err).To(HaveOccurred())
		})

		It("OSボリュームの生成 （失敗ケース)", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("qcow2"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volKey, err = m.CreateVolume(v)
			GinkgoWriter.Println("err=", err)
			GinkgoWriter.Println("Created volume key: ", volKey)
			Expect(err).To(HaveOccurred())
		})

	})

	Context("データボリュームの生成から削除", func() {
		var m *marmotd.Marmot
		var volKey string
		var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATAボリュームの生成", func() {
			v := api.Volume{
				Name: "test-data-volume-001",
				Type: ut.StringPtr("lvm"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(20),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			volKey, err = m.CreateVolume(v)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", volKey)
		})

		It("Dataボリュームの削除", func() {
			err = m.RemoveVolume(volKey)
			Expect(err).NotTo(HaveOccurred())
		})

	})

}
