package marmotd_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("サーバーテスト", Ordered, func() {
	const (
		marmotPort        = 8100
		etcdPort          = 4379
		etcdctlExe        = "/usr/bin/etcdctl"
		nodeName          = "hvc"
		etcdImage         = "ghcr.io/takara9/etcd:3.6.5"
		etcdContainerName = "etcd-server"
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
	)
	etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)
	marmotEp := "localhost:" + fmt.Sprintf("%d", marmotPort)

	BeforeAll(func(ctx0 SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
	})

	AfterAll(func(ctx0 SpecContext) {
		marmotd.CleanupTestEnvironment()
	})

	Context("テスト環境初期化", func() {
		var hvs config.Hypervisors_yaml

		It("モックサーバー用etcdの起動", func() {
			cmd := exec.Command("docker", "run", "-d", "--name", etcdContainerName, "-p", fmt.Sprintf("%d", etcdPort)+":2379", "-p", fmt.Sprintf("%d", etcdPort+1)+":2380", "--rm", etcdImage)
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
			}
			containerID = string(output[:12]) // 最初の12文字をIDとして取得
			fmt.Printf("Container started with ID: %s\n", containerID)
		})

		It("モックサーバーの起動", func() {
			GinkgoWriter.Println("Start marmot server mock")
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
		})

		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://"+marmotEp+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-server.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				fmt.Println(hv)
				err := marmotServer.Ma.Db.SetHypervisors(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := marmotServer.Ma.Db.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のリセット", func() {
			for _, sq := range hvs.Seq {
				err := marmotServer.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", etcdUrl+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})
	})

	Context("最小構成 QCOW2 仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:bootはqcow2 で最小構成", func() {
			var spec api.Server
			var err error
			spec.Name = util.StringPtr("test-vm-1")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			// 他すべてデフォルト
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-1"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("時間待ち", func() {
			time.Sleep(15 * time.Second)
		})

		It("仮想サーバーのOS起動待ち 60秒", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
				g.Expect(*sv.Status).To(Equal(db.SERVER_AVAILABLE))
			}, "120s", "10s").Should(Succeed())
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerById(id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("最小構成 LV 仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:bootはlv で最小構成", func() {
			var spec api.Server
			var err error
			spec.Name = util.StringPtr("test-vm-2")
			spec.BootVolumeType = util.StringPtr("lvm") // ここだけqcow2と違う
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			// 他すべてデフォルト
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-2"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("時間待ち", func() {
			time.Sleep(3 * 60 * time.Second)
		})

		It("仮想サーバーのOS起動待ち 60秒", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
				g.Expect(*sv.Status).To(Equal(db.SERVER_AVAILABLE))
			}, "120s", "10s").Should(Succeed())
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerById(id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("API内部関数テスト", func() {
		It("Marmotd のバージョン情報取得", func() {
			// 中身は未実装
			GinkgoWriter.Println(string("未実装"))
		})
	})

	Context("停止", func() {
		It("コンテナとモック", func() {
			cmd := exec.Command("docker", "kill", containerID)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to stop container: %v\n", err)
			}
			cmd = exec.Command("docker", "rm", containerID)
			_, err = cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to remove container: %v\n", err)
			}
			cancel() // モックサーバー停止
		})
	})
})
