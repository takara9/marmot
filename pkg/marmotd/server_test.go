package marmotd_test

import (
	"context"
	"encoding/json"
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
	ut "github.com/takara9/marmot/pkg/util"
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

		/*
			It("ハイパーバイザーの情報セット", func() {
				for _, hv := range hvs.Hvs {
					fmt.Println(hv)
					err := marmotServer.Ma.Db.SetHypervisors(hv)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		*/

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
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
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

	Context("QCOW2のデータディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:bootはqcow2 でデータディスク２本構成", func() {
			var spec api.Server
			var err error

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			spec.Name = util.StringPtr("test-vm-2")

			By("NICの接続先ネットワークを設定")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}

			By("データディスクのスペックを設定")
			spec.Storage = &[]api.Volume{
				{
					Type: util.StringPtr("qcow2"),
					Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-1"),
					Size: util.IntPtrInt(100), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-2"),
					Size: util.IntPtrInt(200), //MB
				},
			}

			By("他すべてデフォルトで、仮想サーバーを作成")
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバーの取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-2"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
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
			}, "60s", "10s").Should(Succeed())
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
			var vol api.Volume
			spec.Name = util.StringPtr("test-vm-3")
			vol.Type = util.StringPtr("lvm")
			spec.BootVolume = &vol // ここだけqcow2と違う
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			// 他すべてデフォルト
			// この中で、ブートボリュームのIDがセットされていない可能性がある？？？
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバーの取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
			Expect(*sv.Name).To(Equal("test-vm-3"))
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

	Context("LVのデータディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:bootはlv で最小構成", func() {
			var spec api.Server
			var volspec api.Volume
			var err error

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			spec.Name = util.StringPtr("test-vm-4")

			By("NICの接続先ネットワークを設定")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			By("ブートディスクのタイプ(LVM)を設定")
			volspec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
			spec.BootVolume = &volspec
			//spec.BootVolume.Type = util.StringPtr("lvm")

			By("データディスクのスペックを設定")
			spec.Storage = &[]api.Volume{
				{
					Type: util.StringPtr("lvm"),
					Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-1"),
					Size: util.IntPtrInt(100), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-2"),
					Size: util.IntPtrInt(200), //MB
				},
			}

			By("他すべてデフォルトで、仮想サーバーを作成")
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-4"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("LVMサーバーの時間待ち", func() {
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

	Context("LVとQCOW2のデータディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:bootはlv で最小構成", func() {
			var spec api.Server
			var volspec api.Volume
			var err error

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			spec.Name = util.StringPtr("test-vm-5")

			By("NICの接続先ネットワークを設定")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			By("ブートディスクのタイプ(LVM)を設定")
			volspec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
			spec.BootVolume = &volspec

			By("データディスクのスペックを設定")
			spec.Storage = &[]api.Volume{
				{
					Type: util.StringPtr("lvm"),
					Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-1"),
					Size: util.IntPtrInt(100), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-2"),
					Size: util.IntPtrInt(200), //MB
				},
			}

			By("他すべてデフォルトで、仮想サーバーを作成")
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-5"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("LVMサーバーの時間待ち", func() {
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

	Context("qcow2x10データディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:最大構成", func() {
			var spec api.Server
			var volspec api.Volume
			var err error

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			spec.Name = util.StringPtr("test-vm-6")

			By("NICの接続先ネットワークを設定")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			By("ブートディスクのタイプ(LVM)を設定")
			volspec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
			spec.BootVolume = &volspec

			By("データディスクのスペックを設定")
			spec.Storage = &[]api.Volume{
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-1"),
					Size: util.IntPtrInt(101), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-2"),
					Size: util.IntPtrInt(102), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-3"),
					Size: util.IntPtrInt(103), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-4"),
					Size: util.IntPtrInt(104), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-5"),
					Size: util.IntPtrInt(105), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-6"),
					Size: util.IntPtrInt(106), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-7"),
					Size: util.IntPtrInt(107), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-8"),
					Size: util.IntPtrInt(108), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-9"),
					Size: util.IntPtrInt(109), //MB
				},
				{
					Type: util.StringPtr("qcow2"),
					Name: util.StringPtr("data-disk-10"),
					Size: util.IntPtrInt(110), //MB
				},
			}
			By("他すべてデフォルトで、仮想サーバーを作成")
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-6"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("LVMサーバーの時間待ち", func() {
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

	Context("LVx10データディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成 最大最小構成", func() {
			var spec api.Server
			var volspec api.Volume
			var err error

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			spec.Name = util.StringPtr("test-vm-7")

			By("NICの接続先ネットワークを設定")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}
			By("ブートディスクのタイプ(LVM)を設定")
			volspec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
			spec.BootVolume = &volspec

			By("データディスクのスペックを設定")
			spec.Storage = &[]api.Volume{
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-1"),
					Size: util.IntPtrInt(101), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-2"),
					Size: util.IntPtrInt(102), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-3"),
					Size: util.IntPtrInt(103), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-4"),
					Size: util.IntPtrInt(104), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-5"),
					Size: util.IntPtrInt(105), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-6"),
					Size: util.IntPtrInt(106), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-7"),
					Size: util.IntPtrInt(107), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-8"),
					Size: util.IntPtrInt(108), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-9"),
					Size: util.IntPtrInt(109), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					//Kind: util.StringPtr("data"),
					Name: util.StringPtr("data-disk-10"),
					Size: util.IntPtrInt(110), //MB
				},
			}
			By("他すべてデフォルトで、仮想サーバーを作成")
			id, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerById(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal("test-vm-7"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("LVMサーバーの時間待ち", func() {
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

	Context("既存,作成済のデータボリュームをアタッチして起動する仮想サーバーの起動と終了のテスト", func() {
		var serverId string
		hostname := "test-vm-8"
		var volumeIds []string

		It("DATA論理ボリュームの生成1", func() {
			v := api.Volume{
				Name: ut.StringPtr("precreated-volume-001"),
				Size: ut.IntPtrInt(100),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			tmpSpec, err := marmotServer.Ma.CreateNewVolume(v)
			volumeIds = append(volumeIds, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("DATA論理ボリュームの生成2", func() {
			v := api.Volume{
				Name: ut.StringPtr("precreated-volume-002"),
				Size: ut.IntPtrInt(200),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			tmpSpec, err := marmotServer.Ma.CreateNewVolume(v)
			volumeIds = append(volumeIds, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("仮想サーバー生成:bootはqcow2 でデータディスク２本構成", func() {
			var spec api.Server
			var err error

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			spec.Name = util.StringPtr(hostname)

			By("NICの接続先ネットワークを設定")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
			}

			By("データディスクのスペックを設定")
			spec.Storage = &[]api.Volume{
				{
					Id: volumeIds[0],
				},
				{
					Id: volumeIds[1],
				},
			}

			By("他すべてデフォルトで、仮想サーバーを作成")
			serverId, err = marmotServer.Ma.CreateServer(spec)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", serverId)
		})

		It("稼働中仮想サーバーの取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", serverId)
			sv, err := marmotServer.Ma.GetServerById(serverId)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Name)
			Expect(*sv.Name).To(Equal(hostname))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("時間待ち", func() {
			time.Sleep(15 * time.Second)
		})

		It("仮想サーバーのOS起動待ち 60秒", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerById(serverId)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status)
				g.Expect(*sv.Status).To(Equal(db.SERVER_AVAILABLE))
			}, "60s", "10s").Should(Succeed())
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerById(serverId)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ボリュームの削除", func() {
			err := marmotServer.Ma.RemoveVolume(volumeIds[0])
			Expect(err).NotTo(HaveOccurred())
			err = marmotServer.Ma.RemoveVolume(volumeIds[1])
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("複数インターフェース仮想サーバーの起動と終了のテスト", func() {
		var id string
		It("仮想サーバー生成:bootはqcow2 で構成", func() {
			var spec api.Server
			var err error
			spec.Name = util.StringPtr("test-vm-9")
			spec.Network = &[]api.Network{
				{
					Id: "default",
				},
				{
					Id: "host-bridge",
				},
				{
					Id: "ovs-network",
				},
			}
			spec.Storage = &[]api.Volume{
				{
					Type: util.StringPtr("lvm"),
					Name: util.StringPtr("data-disk-1"),
					Size: util.IntPtrInt(101), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					Name: util.StringPtr("data-disk-2"),
					Size: util.IntPtrInt(102), //MB
				},
				{
					Type: util.StringPtr("lvm"),
					Name: util.StringPtr("data-disk-3"),
					Size: util.IntPtrInt(103), //MB
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
			Expect(*sv.Name).To(Equal("test-vm-9"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
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
