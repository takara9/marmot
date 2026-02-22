package marmotd_test

import (
	"context"
	"encoding/json"
	"fmt"
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

var _ = Describe("VirtualPrivateNetworks", Ordered, func() {
	const (
		marmotPort        = 8093
		etcdPort          = 9379
		etcdctlExe        = "etcdctl"
		nodeName          = "hvc"
		etcdImage         = "ghcr.io/takara9/etcd:3.6.5"
		etcdContainerName = "etcd-net"
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
	)
	etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)

	BeforeAll(func(ctx0 SpecContext) {
	})

	AfterAll(func(ctx0 SpecContext) {
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
			time.Sleep(10 * time.Second) // コンテナが起動するまで待機
		})

		It("モックサーバーの起動", func() {
			GinkgoWriter.Println("Start marmot server mock")
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
		})

		var hvs config.Hypervisors_yaml
		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc-func.yaml", &hvs)
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

		It("シーケンス番号のセット", func() {
			for _, sq := range hvs.Seq {
				err := marmotServer.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("起動完了待ちチェック", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", etcdUrl+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		/*
			It("動作確認 CheckHypervisors()", func() {
				GinkgoWriter.Println(nodeName)
				hv, err := marmotServer.Ma.Db.CheckHypervisors(etcdUrl, nodeName)
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
		*/

		It("Check the config file to directly etcd", func() {
			cmd := exec.Command(etcdctlExe, "--endpoints=localhost:9379", "get", "hvc")
			cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
			out, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(out))
			Expect(err).To(Succeed()) // 成功
		})
	})

	// 最初はコントローラーなしでテストなので、
	// etcdに反映されることを確認するまでとする。
	Context("仮想ネットワークの生成から削除", func() {
		var m *marmotd.Marmot
		var networkId1, networkId2 string
		//var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))
			Expect(err).NotTo(HaveOccurred())
		})

		It("最小スペックの仮想ネットワークの生成", func() {
			var net api.VirtualNetwork
			var meta api.Metadata
			var spec api.VirtualNetworkSpec
			net.Metadata = &meta
			net.Spec = &spec
			net.Metadata.Name = util.StringPtr("test-network")
			net.Spec.IpAddress = util.StringPtr("192.168.200.0/24")
			createdNet, err := m.Db.CreateVirtualNetwork(net)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdNet.Id).NotTo(BeEmpty())
			Expect(createdNet.Metadata.Uuid).NotTo(BeNil())
			Expect(createdNet.Spec.IpAddress).To(Equal(util.StringPtr("192.168.200.0/24")))
			GinkgoWriter.Println("Created network ID: ", createdNet.Id)
			networkId1 = createdNet.Id
		})

		It("仮想ネットワークの情報取得", func() {
			net, err := m.Db.GetVirtualNetworkById(networkId1)
			Expect(err).NotTo(HaveOccurred())
			Expect(net).NotTo(BeNil())
			jsonByte, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

		It("同一サブネットの仮想ネットワークの生成", func() {
			var net api.VirtualNetwork
			var meta api.Metadata
			var spec api.VirtualNetworkSpec
			net.Metadata = &meta
			net.Spec = &spec
			net.Metadata.Name = util.StringPtr("test-network2")
			net.Spec.IpAddress = util.StringPtr("192.168.200.0/24")
			createdNet, err := m.Db.CreateVirtualNetwork(net)
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Println("Created network ID: ", createdNet.Id)
			networkId2 = createdNet.Id
		})

		It("仮想ネットワークのステータス更新", func() {
			m.Db.UpdateVirtualNetworkStatus(networkId1, db.NETWORK_ACTIVE)
		})

		It("仮想ネットワークの情報取得", func() {
			net, err := m.Db.GetVirtualNetworkById(networkId1)
			Expect(err).NotTo(HaveOccurred())
			Expect(net).NotTo(BeNil())
			jsonByte, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

		It("仮想ネットワークのステータス更新", func() {
			m.Db.UpdateVirtualNetworkStatus(networkId2, db.NETWORK_INACTIVE)
		})

		It("稼働ネットワークの情報更新", func() {
			var net api.VirtualNetwork
			var spec api.VirtualNetworkSpec
			net.Spec = &spec
			net.Spec.Dhcp = util.BoolPtr(true)
			err := m.Db.UpdateVirtualNetworkById(networkId1, net)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークの情報取得", func() {
			net, err := m.Db.GetVirtualNetworkById(networkId1)
			Expect(err).NotTo(HaveOccurred())
			Expect(net).NotTo(BeNil())
			jsonByte, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

		It("仮想ネットワークの情報取得", func() {
			net, err := m.Db.GetVirtualNetworkById(networkId2)
			Expect(err).NotTo(HaveOccurred())
			Expect(net).NotTo(BeNil())
			jsonByte, err := json.MarshalIndent(net, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

		It("同一ネットワーク名の仮想ネットワークの生成", func() {
			var net api.VirtualNetwork
			var meta api.Metadata
			var spec api.VirtualNetworkSpec
			net.Metadata = &meta
			net.Spec = &spec
			net.Metadata.Name = util.StringPtr("test-network2")
			net.Spec.IpAddress = util.StringPtr("192.168.201.0/24")
			createdNet, err := m.Db.CreateVirtualNetwork(net)
			Expect(err).To(HaveOccurred())
			Expect(createdNet.Id).To(BeEmpty())
			GinkgoWriter.Println("Created network ID: ", createdNet.Id)
		})

		It("仮想ネットワークの削除タイムスタンプのセット", func() {
			err := m.Db.SetDeleteTimestampVirtualNetwork(networkId1)
			Expect(err).NotTo(HaveOccurred())
			err = m.Db.SetDeleteTimestampVirtualNetwork(networkId2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークのリストの取得 削除中", func() {
			nets, err := m.Db.GetVirtualNetworks()
			Expect(err).NotTo(HaveOccurred())
			Expect(nets).NotTo(BeNil())
			jsonByte, err := json.MarshalIndent(nets, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

		It("仮想ネットワークの削除", func() {
			err := m.Db.DeleteVirtualNetworkById(networkId1)
			Expect(err).NotTo(HaveOccurred())
			err = m.Db.DeleteVirtualNetworkById(networkId2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想ネットワークのリストの取得", func() {
			nets, err := m.Db.GetVirtualNetworks()
			Expect(err).NotTo(HaveOccurred())
			Expect(nets).To(BeNil())
			for _, net := range nets {
				GinkgoWriter.Println("ネットワークID: ", net.Id, " ネットワーク名: ", *net.Metadata.Name, " ステータス: ", db.NetworkStatus[*net.Status.Status])
				Expect(net.Status.DeletionTimeStamp).NotTo(BeNil())
			}
			jsonByte, err := json.MarshalIndent(nets, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

	})

	Context("コンテナとモックの停止", func() {
		It("停止コマンド実行", func() {
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
