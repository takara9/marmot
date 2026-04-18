package marmotd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("ServerImageCopyingTest", Ordered, func() {
	const (
		marmotPort        = 8102
		etcdPort          = 14379
		etcdctlExe        = "/usr/bin/etcdctl"
		nodeName          = "hvc"
		etcdImage         = "ghcr.io/takara9/etcd:3.6.5"
		etcdContainerName = "etcd-server-14379"
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
		osImageid    string
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
		err := marmotServer.Ma.DeleteImageManage(osImageid)
		Expect(err).NotTo(HaveOccurred())
		_, err = marmotServer.Ma.GetImageManage(osImageid)
		Expect(err).To(HaveOccurred())
		fmt.Println("Deleted image ID: ", osImageid)

		cmd := exec.Command("docker", "kill", containerID)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerID)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}

		cancel() // モックサーバー停止
		marmotd.CleanupTestEnvironment()
	})

	Context("テスト環境初期化", func() {
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

		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", etcdUrl+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})
		It("既存ネットワークの取得", func() {
			var err error
			vnets, err := marmotServer.Ma.GetVirtualNetworksAndPutDB()
			Expect(err).NotTo(HaveOccurred())
			for _, name := range vnets {
				byteJson, err := json.MarshalIndent(name, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Found Network:", string(byteJson))
			}
		})
	})

	Context("イメージ作成", func() {
		It("URLを指定してイメージのIDを取得 DB操作のみ", func() {
			var err error
			GinkgoWriter.Println("URLを指定してイメージのIDを取得")
			url := "http://hmc/ubuntu-22.04-server-cloudimg-amd64.img"
			osImageid, err = marmotServer.Ma.Db.MakeImageEntryFromURL("ubuntu22.04", url)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("取得したイメージID: ", osImageid)
		})
		It("イメージのセットアップ", func() {
			image, err := marmotServer.Ma.CreateNewImageManage(osImageid)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image: ", string(jsonBytes))
		})
	})

	Context("QCOW2 仮想サーバー起動と終了", func() {
		It("仮想ネットワークの取得", func() {
			net, err := marmotServer.Ma.Db.GetVirtualNetworks()
			Expect(err).NotTo(HaveOccurred())
			data, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("ネットワーク情報: ", string(data))
		})

		var id string
		It("仮想サーバーqcow2 起動", func() {
			var virtualServer api.Server
			var meta api.Metadata
			var spec api.ServerSpec
			var err error
			meta.Name = util.StringPtr("test-vm-1")
			virtualServer.Metadata = &meta
			virtualServer.Spec = &spec
			virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
				{
					Networkname: "default",
				},
			}
			// 他すべてデフォルト
			vm, err := marmotServer.Ma.Db.MakeServerEntry(virtualServer)
			Expect(err).NotTo(HaveOccurred())
			id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("起動チェック", func() {
			GinkgoWriter.Println("仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerManage(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			Expect(*sv.Metadata.Name).To(Equal("test-vm-1"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("オブジェクト情報: ", string(data))
		})

		It("OS起動待ち", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}, "300s", "10s").Should(Succeed())
		})

		It("削除", func() {
			err := marmotServer.Ma.DeleteServerByIdManage(id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("イメージ取得のテスト", func() {
		var id string
		It("仮想サーバー起動 boot + data x2 qcow2", func() {
			var virtualServer api.Server
			var meta api.Metadata
			var spec api.ServerSpec
			var err error
			virtualServer.Spec = &spec

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			meta.Name = util.StringPtr("test-vm-image-1")
			virtualServer.Metadata = &meta

			By("NICの接続先ネットワークを設定")
			virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
				{
					Networkname: "default",
				},
			}

			By("データディスクのスペックを設定")
			virtualServer.Spec.Storage = &[]api.Volume{
				{
					Metadata: &api.Metadata{
						Name: util.StringPtr("data-disk-1"),
					},
					Spec: &api.VolSpec{
						Type: util.StringPtr("qcow2"),
						Kind: util.StringPtr("data"),
						Size: util.IntPtrInt(100), //MB
					},
				},
				{
					Metadata: &api.Metadata{
						Name: util.StringPtr("data-disk-2"),
					},
					Spec: &api.VolSpec{
						Type: util.StringPtr("qcow2"),
						Kind: util.StringPtr("data"),
						Size: util.IntPtrInt(200), //MB
					},
				},
			}

			By("他すべてデフォルトで、仮想サーバーを作成")
			vm, err := marmotServer.Ma.Db.MakeServerEntry(virtualServer)
			Expect(err).NotTo(HaveOccurred())
			id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("起動チェック", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerManage(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			Expect(*sv.Metadata.Name).To(Equal("test-vm-image-1"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("オブジェクト情報: ", string(data))
		})

		It("OS起動待ち", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}, "600s", "10s").Should(Succeed())
		})

		It("イメージの作成", func() {
			img, err := marmotServer.Ma.Db.MakeImageEntryFromRunningVM(id, "image-1")
			Expect(err).NotTo(HaveOccurred())
			imageId, err := marmotServer.Ma.MakeImageEntryFromRunningVM(id, "image-1", img)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created image ID: ", imageId)
			image, err := marmotServer.Ma.Db.GetImage(imageId)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created image: ", *image.Metadata.Name)
			data, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image details: ", string(data))
		})

		It("削除", func() {
			err := marmotServer.Ma.DeleteServerByIdManage(id)
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("image-1(qcow2) からの起動テスト", func() {
		It("仮想ネットワークの取得", func() {
			net, err := marmotServer.Ma.Db.GetVirtualNetworks()
			Expect(err).NotTo(HaveOccurred())
			data, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("ネットワーク情報: ", string(data))
		})

		var id string
		It("qcow2イメージからの仮想サーバー起動", func() {
			var virtualServer api.Server
			var meta api.Metadata
			var spec api.ServerSpec
			var err error
			meta.Name = util.StringPtr("test-vm-2")
			virtualServer.Metadata = &meta
			virtualServer.Spec = &spec
			virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
				{
					Networkname: "default",
				},
			}
			virtualServer.Spec.OsVariant = util.StringPtr("image-1")
			// 他すべてデフォルト
			vm, err := marmotServer.Ma.Db.MakeServerEntry(virtualServer)
			Expect(err).NotTo(HaveOccurred())
			id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("起動チェック", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerManage(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			Expect(*sv.Metadata.Name).To(Equal("test-vm-2"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
		})

		It("OS起動待ち", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}, "180s", "10s").Should(Succeed())
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerByIdManage(id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("LV仮想サーバー", func() {
		var id string
		It("起動", func() {
			var virtualServer api.Server
			var meta api.Metadata
			var spec api.ServerSpec
			virtualServer.Metadata = &meta
			virtualServer.Spec = &spec

			var bootVol api.Volume
			var specVol api.VolSpec
			var metaVol api.Metadata
			bootVol.Metadata = &metaVol
			bootVol.Spec = &specVol
			virtualServer.Spec.BootVolume = &bootVol
			var err error

			virtualServer.Metadata.Name = util.StringPtr("test-vm-3")
			virtualServer.Spec.BootVolume.Spec.Type = util.StringPtr("lvm")
			virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
				{
					Networkname: "default",
				},
			}

			// 他すべてデフォルト
			vm, err := marmotServer.Ma.Db.MakeServerEntry(virtualServer)
			Expect(err).NotTo(HaveOccurred())
			id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("起動チェック", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerManage(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
			Expect(*sv.Metadata.Name).To(Equal("test-vm-3"))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
		})

		It("OS起動待ち", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}, "180s", "10s").Should(Succeed())
		})

		It("LVイメージ取得", func() {
			img, err := marmotServer.Ma.Db.MakeImageEntryFromRunningVM(id, "image-2")
			Expect(err).NotTo(HaveOccurred())
			imageId, err := marmotServer.Ma.MakeImageEntryFromRunningVM(id, "image-2", img)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created image ID: ", imageId)
			image, err := marmotServer.Ma.Db.GetImage(imageId)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created image: ", *image.Metadata.Name)
			data, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image details: ", string(data))
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerByIdManage(id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("image-2(LV) からの起動テスト", func() {
		It("仮想ネットワークの取得", func() {
			net, err := marmotServer.Ma.Db.GetVirtualNetworks()
			Expect(err).NotTo(HaveOccurred())
			data, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("ネットワーク情報: ", string(data))
		})

		var id string
		It("LV仮想サーバー起動", func() {
			var err error
			var virtualServer api.Server
			var meta api.Metadata
			var spec api.ServerSpec
			virtualServer.Metadata = &meta
			virtualServer.Spec = &spec
			var bootVol api.Volume
			var specVol api.VolSpec
			var metaVol api.Metadata
			bootVol.Metadata = &metaVol
			bootVol.Spec = &specVol

			virtualServer.Spec.BootVolume = &bootVol

			meta.Name = util.StringPtr("test-vm-2")
			virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
				{
					Networkname: "default",
				},
			}
			virtualServer.Spec.OsVariant = util.StringPtr("image-2")
			virtualServer.Spec.BootVolume.Spec.Type = util.StringPtr("lvm")
			// 他すべてデフォルト

			vm, err := marmotServer.Ma.Db.MakeServerEntry(virtualServer)
			Expect(err).NotTo(HaveOccurred())
			id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", id)
		})

		It("起動チェック", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", id)
			sv, err := marmotServer.Ma.GetServerManage(id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			Expect(*sv.Metadata.Name).To(Equal("test-vm-2"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
		})

		It("OS起動待ち", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}, "180s", "10s").Should(Succeed())
		})

		It("仮想サーバーの削除", func() {
			marmotServer.Ma.DeleteServerByIdManage(id)
			//Expect(err).NotTo(HaveOccurred())
		})
	})

	/*
		Context("LVのデータディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
			var id string
			It("仮想サーバー生成:bootはlv で最小構成", func() {
				var virtualServer api.Server
				var meta api.Metadata
				var spec api.ServerSpec
				var err error
				virtualServer.Spec = &spec

				var bootVol api.Volume
				var specVol api.VolSpec
				var metaVol api.Metadata
				bootVol.Metadata = &metaVol
				bootVol.Spec = &specVol

				By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
				meta.Name = util.StringPtr("test-vm-4")
				virtualServer.Metadata = &meta

				By("NICの接続先ネットワークを設定")
				virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
					{
						Networkname: "default",
					},
				}

				By("ブートディスクのタイプ(LVM)を設定")
				bootVol.Spec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
				virtualServer.Spec.BootVolume = &bootVol

				By("データディスクのスペックを設定")
				virtualServer.Spec.Storage = &[]api.Volume{
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-1"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-2"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(2), //GB
						},
					},
				}

				By("他すべてデフォルトで、仮想サーバーを作成")
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err = marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred()) //////////////////////// ここで失敗　続きはここから
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバー（１）の取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", id)
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				Expect(*sv.Metadata.Name).To(Equal("test-vm-4"))
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
			})

			// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
			It("LVMサーバーの時間待ち", func() {
				time.Sleep(15 * time.Second)
			})

			It("仮想サーバーのOS起動待ち 60秒", func() {
				Eventually(func(g Gomega) {
					sv, err := marmotServer.Ma.GetServerById(id)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
					g.Expect(*sv.Status.Status).To(Equal(db.SERVER_RUNNING))
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
				var virtualServer api.Server
				var meta api.Metadata
				var spec api.ServerSpec
				var err error
				virtualServer.Spec = &spec
				virtualServer.Metadata = &meta

				var bootVol api.Volume
				var specVol api.VolSpec
				var metaVol api.Metadata
				bootVol.Metadata = &metaVol
				bootVol.Spec = &specVol

				By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
				virtualServer.Metadata.Name = util.StringPtr("test-vm-5")

				By("NICの接続先ネットワークを設定")
				virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
					{
						Networkname: "default",
					},
				}

				By("ブートディスクのタイプ(LVM)を設定")
				bootVol.Spec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
				virtualServer.Spec.BootVolume = &bootVol

				By("データディスクのスペックを設定")
				virtualServer.Spec.Storage = &[]api.Volume{
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-1"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-2"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
				}

				By("他すべてデフォルトで、仮想サーバーを作成")
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err = marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバー（１）の取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", id)
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				Expect(*sv.Metadata.Name).To(Equal("test-vm-5"))
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
			})

			// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
			It("LVMサーバーの時間待ち", func() {
				time.Sleep(15 * time.Second)
			})

			It("仮想サーバーのOS起動待ち 60秒", func() {
				Eventually(func(g Gomega) {
					sv, err := marmotServer.Ma.GetServerById(id)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
					g.Expect(*sv.Status.Status).To(Equal(db.SERVER_RUNNING))
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
				var err error
				var virtualServer api.Server
				var meta api.Metadata
				var spec api.ServerSpec
				virtualServer.Spec = &spec

				var bootVol api.Volume
				var specVol api.VolSpec
				var metaVol api.Metadata
				bootVol.Metadata = &metaVol
				bootVol.Spec = &specVol
				virtualServer.Spec.BootVolume = &bootVol

				By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
				meta.Name = util.StringPtr("test-vm-6")
				virtualServer.Metadata = &meta

				By("NICの接続先ネットワークを設定")
				virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
					{
						Networkname: "default",
					},
				}
				By("ブートディスクのタイプ(LVM)を設定")
				bootVol.Spec.Type = util.StringPtr("lvm") // ここだけqcow2と違う
				virtualServer.Spec.BootVolume = &bootVol

				By("データディスクのスペックを設定")
				virtualServer.Spec.Storage = &[]api.Volume{
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-1"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-2"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-3"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-4"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-5"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-6"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-7"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-8"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-9"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-10"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("qcow2"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
				}
				By("他すべてデフォルトで、仮想サーバーを作成")
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err = marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバー（１）の取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", id)
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				Expect(*sv.Metadata.Name).To(Equal("test-vm-6"))
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
			})

			// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
			It("LVMサーバーの時間待ち", func() {
				time.Sleep(15 * time.Second)
			})

			It("仮想サーバーのOS起動待ち 60秒", func() {
				Eventually(func(g Gomega) {
					sv, err := marmotServer.Ma.GetServerById(id)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
					g.Expect(*sv.Status.Status).To(Equal(db.SERVER_RUNNING))
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
				var virtualServer api.Server
				var meta api.Metadata
				var spec api.ServerSpec
				var err error
				virtualServer.Spec = &spec

				var bootVol api.Volume
				var specVol api.VolSpec
				var metaVol api.Metadata
				bootVol.Metadata = &metaVol
				bootVol.Spec = &specVol
				virtualServer.Spec.BootVolume = &bootVol

				By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
				meta.Name = util.StringPtr("test-vm-7")
				virtualServer.Metadata = &meta

				By("NICの接続先ネットワークを設定")
				virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
					{
						Networkname: "default",
					},
				}
				By("ブートディスクのタイプ(LVM)を設定")
				bootVol.Spec.Type = util.StringPtr("lvm") // ここだけqcow2と違う

				By("データディスクのスペックを設定")
				virtualServer.Spec.Storage = &[]api.Volume{
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-1"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-2"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-3"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-4"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-5"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-6"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-7"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-8"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-9"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-10"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
				}
				By("他すべてデフォルトで、仮想サーバーを作成")
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err = marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバー（１）の取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", id)
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				Expect(*sv.Metadata.Name).To(Equal("test-vm-7"))
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
					GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
					g.Expect(*sv.Status.Status).To(Equal(db.SERVER_RUNNING))
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
					Metadata: &api.Metadata{
						Name: ut.StringPtr("precreated-volume-001"),
					},
					Spec: &api.VolSpec{
						Size: ut.IntPtrInt(100),
					},
				}
				GinkgoWriter.Println("Creating Data volume", "volume", v)
				tmpSpec, err := marmotServer.Ma.CreateNewVolumeWithWait(v)
				volumeIds = append(volumeIds, tmpSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created volume key: ", *tmpSpec.Metadata.Key)
			})

			It("DATA論理ボリュームの生成2", func() {
				v := api.Volume{
					Metadata: &api.Metadata{
						Name: ut.StringPtr("precreated-volume-002"),
					},
					Spec: &api.VolSpec{
						Size: ut.IntPtrInt(200),
					},
				}
				GinkgoWriter.Println("Creating Data volume", "volume", v)
				tmpSpec, err := marmotServer.Ma.CreateNewVolumeWithWait(v)
				volumeIds = append(volumeIds, tmpSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created volume key: ", *tmpSpec.Metadata.Key)
			})

			It("仮想サーバー生成:bootはqcow2 でデータディスク２本構成", func() {
				var virtualServer api.Server
				var meta api.Metadata
				var spec api.ServerSpec
				virtualServer.Spec = &spec

				var err error

				By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
				meta.Name = util.StringPtr(hostname)
				virtualServer.Metadata = &meta

				By("NICの接続先ネットワークを設定")
				virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
					{
						Networkname: "default",
					},
				}

				By("データディスクのスペックを設定")
				virtualServer.Spec.Storage = &[]api.Volume{
					{
						Id: volumeIds[0],
					},
					{
						Id: volumeIds[1],
					},
				}

				By("他すべてデフォルトで、仮想サーバーを作成")
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err := marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred())
				serverId = vm.Id
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバーの取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", serverId)
				sv, err := marmotServer.Ma.GetServerById(serverId)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				Expect(*sv.Metadata.Name).To(Equal(hostname))
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
			})

			// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
			It("時間待ち", func() {
				time.Sleep(15 * time.Second)
			})

			It("仮想サーバーのOS起動待ち 60秒", func() {
				Eventually(func(g Gomega) {
					sv, err := marmotServer.Ma.GetServerById(serverId)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
					g.Expect(*sv.Status.Status).To(Equal(db.SERVER_RUNNING))
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
				var virtualServer api.Server
				var meta api.Metadata
				var spec api.ServerSpec
				virtualServer.Spec = &spec
				var err error

				meta.Name = util.StringPtr("test-vm-9")
				virtualServer.Metadata = &meta
				virtualServer.Spec.NetworkInterface = &[]api.NetworkInterface{
					{
						Networkname: "default",
					},
					{
						Networkname: "host-bridge",
					},
					{
						Networkname: "ovs-network",
					},
				}
				spec.Storage = &[]api.Volume{
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-1"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-2"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
					{
						Metadata: &api.Metadata{
							Name: util.StringPtr("data-disk-3"),
						},
						Spec: &api.VolSpec{
							Type: util.StringPtr("lvm"),
							Kind: util.StringPtr("data"),
							Size: util.IntPtrInt(1), //GB
						},
					},
				}

				// 他すべてデフォルト
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err = marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバー（１）の取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", id)
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				Expect(*sv.Metadata.Name).To(Equal("test-vm-9"))
				data, err := json.MarshalIndent(sv, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("サーバー情報: ", string(data))
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
			})

			// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
			It("時間待ち", func() {
				time.Sleep(15 * time.Second)
			})

			It("仮想サーバーのOS起動待ち 60秒", func() {
				Eventually(func(g Gomega) {
					sv, err := marmotServer.Ma.GetServerById(id)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
					g.Expect(*sv.Status.Status).To(Equal(db.SERVER_RUNNING))
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
	*/

})
