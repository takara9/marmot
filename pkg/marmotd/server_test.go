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
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("サーバーテスト", Ordered, func() {
	const (
		marmotPort = 8100
		etcdPort   = 4379
		etcdctlExe = "/usr/bin/etcdctl"
		nodeName   = "hvc"
		etcdImage  = "ghcr.io/takara9/etcd:3.6.5"
		osImage    = "ubuntu-22.04-server-cloudimg-amd64.img"
		osImageURL = "http://hmc/" + osImage
	)
	var (
		containerID      string
		ctx              context.Context
		cancel           context.CancelFunc
		marmotServer     *marmotd.Server
		waitServerDone   func()
		osImageid        string
		vm1Id            string
		vm1BootVolumeId  string
		vm2Id            string
		vm2BootVolumeId  string
		vm2DataVolumeId1 string
		vm2DataVolumeId2 string
	)
	etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)
	marmotEp := "localhost:" + fmt.Sprintf("%d", marmotPort)

	BeforeAll(func(ctx0 SpecContext) {
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%d", etcdPort)+":2379", "--rm", etcdImage)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)

		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
	})

	AfterAll(func(ctx0 SpecContext) {

		By("Dockerコンテナの停止とモックサーバーのシャットダウン")
		cmd := exec.Command("docker", "kill", containerID)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cancel() // モックサーバー停止シグナル
		if waitServerDone != nil {
			waitServerDone() // goroutine の終了を待つ
		}
		// /var/lib/marmot/images/9e24c/ubuntu-22.04-server-cloudimg-amd64.img のようなファイルを削除する

		// 以下の後処理は、本来、それぞれの削除関数の中で実施するべきもの
		// OS論理ボリューム vg1/boot-9e24c のようなものができるはずなので、削除する
		By("後処理: 作成された論理ボリュームの削除")
		cmd = exec.Command("lvremove", "-y", "vg1/boot-"+osImageid)
		if err := cmd.Run(); err != nil {
			fmt.Printf("Failed to remove logical volume: %v\n", err)
		}

		// "/var/lib/marmot/images/" + osImageid　のディレクトリを削除する
		By("後処理: 作成されたイメージファイルの削除")
		imagePath := "/var/lib/marmot/images/" + osImageid
		if _, err := os.Stat(imagePath); err == nil {
			if err := os.RemoveAll(imagePath); err != nil {
				fmt.Printf("Failed to remove image directory: %v\n", err)
			}
		}

		// /var/lib/marmot/isos/ の中にディレクトリができるはずなので、削除する
		By("後処理: /var/lib/marmot/isos/ の中に作成されたディレクトリの削除")
		cdimagePath := "/var/lib/marmot/isos/" + vm1Id
		err = os.RemoveAll(cdimagePath)
		Expect(err).NotTo(HaveOccurred())

		cdimagePath = "/var/lib/marmot/isos/" + vm2Id
		err = os.RemoveAll(cdimagePath)
		Expect(err).NotTo(HaveOccurred())

		// /var/lib/marmot/volumesの下に作成されたファイルを消す
		By("後処理: /var/lib/marmot/volumesの下に作成されたファイルの削除 未実装")
		pathVol := "/var/lib/marmot/volumes/" + fmt.Sprintf("boot-%s.qcow2", vm1BootVolumeId)
		err = os.Remove(pathVol)
		Expect(err).NotTo(HaveOccurred())

		pathVol = "/var/lib/marmot/volumes/" + fmt.Sprintf("boot-%s.qcow2", vm2BootVolumeId)
		err = os.Remove(pathVol)
		Expect(err).NotTo(HaveOccurred())

		pathVol = "/var/lib/marmot/volumes/" + fmt.Sprintf("data-%s.qcow2", vm2DataVolumeId1)
		err = os.Remove(pathVol)
		Expect(err).NotTo(HaveOccurred())

		pathVol = "/var/lib/marmot/volumes/" + fmt.Sprintf("data-%s.qcow2", vm2DataVolumeId2)
		err = os.Remove(pathVol)
		Expect(err).NotTo(HaveOccurred())

	})

	Context("テスト環境初期化", func() {
		It("モックサーバーの起動", func() {
			GinkgoWriter.Println("Start marmot server mock")
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer, waitServerDone = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
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

	Context("URLを指定してダウンロードしたイメージからVM起動イメージを作成する", func() {
		It("URLを指定してイメージのIDを取得", func() {
			var err error
			GinkgoWriter.Println("URLを指定してイメージのIDを取得")
			osImageid, err = marmotServer.Ma.Db.MakeImageEntryFromURLWithNode("ubuntu22.04", osImageURL, nodeName)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("取得したイメージID: ", osImageid)
		})

		It("ダウンロードとセットアップ", func() {
			image, err := marmotServer.Ma.CreateNewImageManage(osImageid)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image: ", string(jsonBytes))
		})
	})

	Context("最小構成 QCOW2 仮想サーバーの起動と終了のテスト", func() {
		It("仮想ネットワークの取得", func() {
			net, err := marmotServer.Ma.Db.GetVirtualNetworks()
			Expect(err).NotTo(HaveOccurred())
			data, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("ネットワーク情報: ", string(data))
		})

		//var id string
		It("仮想サーバー生成:bootはqcow2 で最小構成", func() {
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
			vm1Id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", vm1Id)
		})

		It("稼働中仮想サーバー（１）の取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", vm1Id)
			sv, err := marmotServer.Ma.GetServerManage(vm1Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			Expect(*sv.Metadata.Name).To(Equal("test-vm-1"))
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
				sv, err := marmotServer.Ma.GetServerManage(vm1Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
				vm1BootVolumeId = sv.Spec.BootVolume.Id
			}, "120s", "10s").Should(Succeed())
		})

		It("ブートボリュームの metadata.nodeName を保持する", func() {
			bootVol, err := marmotServer.Ma.GetVolumeById(vm1BootVolumeId)
			Expect(err).NotTo(HaveOccurred())
			Expect(bootVol.Metadata).NotTo(BeNil())
			Expect(bootVol.Metadata.NodeName).NotTo(BeNil())
			Expect(*bootVol.Metadata.NodeName).To(Equal(nodeName))
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerByIdManage(vm1Id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("QCOW2のデータディスクが複数存在する仮想サーバーの起動と終了のテスト", func() {
		It("仮想サーバー生成:bootはqcow2 でデータディスク２本構成", func() {
			var virtualServer api.Server
			var meta api.Metadata
			var spec api.ServerSpec
			var err error
			virtualServer.Spec = &spec

			By("仮想サーバーのホスト名を設定、OSへの設定は未実装")
			meta.Name = util.StringPtr("test-vm-2")
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
			vm2Id, err = marmotServer.Ma.CreateServerManage(vm.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created VM ID:", vm2Id)
		})

		// 本来ならばSSHログイン成功まで待ちたい、DHCPとDNSが必要
		It("時間待ち", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(vm2Id)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(sv.Status).NotTo(BeNil())
				g.Expect(sv.Status.StatusCode).NotTo(Equal(db.SERVER_ERROR))
			}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("仮想サーバーのOS起動待ち 60秒", func() {
			Eventually(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(vm2Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}, "60s", "10s").Should(Succeed())
		})

		It("稼働中仮想サーバーの取得", func() {
			GinkgoWriter.Println("取得する仮想サーバーID:", vm2Id)
			sv, err := marmotServer.Ma.GetServerManage(vm2Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
			Expect(*sv.Metadata.Name).To(Equal("test-vm-2"))
			data, err := json.MarshalIndent(sv, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("サーバー情報: ", string(data))
			GinkgoWriter.Println("サーバーステータス: ", *sv.Status.Status)
			vm2BootVolumeId = sv.Spec.BootVolume.Id
			vm2DataVolumeId1 = (*sv.Spec.Storage)[0].Id
			vm2DataVolumeId2 = (*sv.Spec.Storage)[1].Id
		})

		It("LVMの状態確認", func() {
			out, err := exec.Command("lvs", "vg1").Output()
			fmt.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("VGの状態確認", func() {
			out, err := exec.Command("vgs").Output()
			fmt.Println("vgs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("時間待ち PD", func() {
			Consistently(func(g Gomega) {
				sv, err := marmotServer.Ma.GetServerManage(vm2Id)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(sv.Status).NotTo(BeNil())
				g.Expect(sv.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
			}).WithTimeout(20 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("仮想サーバーの削除", func() {
			err := marmotServer.Ma.DeleteServerByIdManage(vm2Id)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	/*
		Context("最小構成 LV 仮想サーバーの起動と終了のテスト", func() {
			var id string
			It("仮想サーバー生成:bootはlv で最小構成", func() {
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
				// この中で、ブートボリュームのIDがセットされていない可能性がある？？？
				vm, err := marmotServer.Ma.Db.CreateServer(virtualServer)
				Expect(err).NotTo(HaveOccurred())
				id, err = marmotServer.Ma.CreateServer2(vm.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("Created VM ID:", id)
			})

			It("稼働中仮想サーバーの取得", func() {
				GinkgoWriter.Println("取得する仮想サーバーID:", id)
				sv, err := marmotServer.Ma.GetServerById(id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("サーバー名: ", *sv.Metadata.Name)
				data, err := json.MarshalIndent(sv, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("サーバー情報: ", string(data))
				Expect(*sv.Metadata.Name).To(Equal("test-vm-3"))
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

	/*
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
				cancel() // モックサーバー停止シグナル
				if waitServerDone != nil {
					waitServerDone() // goroutine の終了を待つ
				}
			})
		})
	*/
})
