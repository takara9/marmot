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
		containerID    string
		ctx            context.Context
		cancel         context.CancelFunc
		waitServerDone func()
		//marmotServer *marmotd.Server
	)
	etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)

	BeforeAll(func(ctx0 SpecContext) {
		By("開始 etcd container")
		cmd := exec.Command("docker", "run", "-d", "-p", fmt.Sprintf("%d", etcdPort)+":2379", "--rm", etcdImage)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)
		time.Sleep(10 * time.Second) // コンテナが起動するまで待機

		By("モックサーバー起動")
		ctx, cancel = context.WithCancel(context.Background())
		_, waitServerDone = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
	})

	AfterAll(func(ctx0 SpecContext) {
		By("停止 etcd container")
		cmd := exec.Command("docker", "kill", containerID)
		_, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())

		By("モックサーバー停止")
		cancel()         // モックサーバー停止シグナル
		waitServerDone() // goroutine の終了を待つ
	})

	Context("テスト環境初期化", func() {
		It("モック起動待ち ＆ チェック", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", etcdUrl+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

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
				GinkgoWriter.Println("ネットワークID: ", net.Id, " ネットワーク名: ", *net.Metadata.Name, " ステータス: ", db.NetworkStatus[net.Status.StatusCode])
				Expect(net.Status.DeletionTimeStamp).NotTo(BeNil())
			}
			jsonByte, err := json.MarshalIndent(nets, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(jsonByte))
		})

	})
})
