package db_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Networks", Ordered, func() {
	var url string = "http://127.0.0.1:8379"
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "jobEtcdDb", "-p", "8379:2379", "-p", "8380:2380", "ghcr.io/takara9/etcd:3.6.5")
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
		fmt.Println("STOPPING CONTAINER:", containerID)
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

	Describe("ネットワーク管理テスト", func() {
		var v *db.Database
		var netSpec api.VirtualNetwork
		var nets []api.VirtualNetwork
		Context("基本アクセス", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ネットワークの作成 #1", func() {
				net := &api.VirtualNetwork{
					Metadata: &api.Metadata{
						Name: util.StringPtr("net01"),
					},
					Spec: &api.VlanSpec{
						BridgeName:       util.StringPtr("br01"),
						DhcpEndAddress:   util.StringPtr("192.168.122.0"),
						DhcpStartAddress: util.StringPtr("192.168.122.254"),
						MacAddress:       util.StringPtr("52:54:00:6b:3c:58"),
						ForwardMode:      util.StringPtr("NAT"),
					},
				}
				netSpec, err = v.CreateVirtualNetwork(*net)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created network with ID:", netSpec.Id)
			})

			It("Keyからネットワーク情報を取得", func() {
				GinkgoWriter.Printf("ネットワークID: %s\n", netSpec.Id)
				net, err := v.GetVirtualNetworkById(netSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				bytes, err := json.MarshalIndent(net, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(bytes))
			})

			It("ネットワークの状態更新 #1", func() {
				net := api.VirtualNetwork{
					Status: &api.Status{
						Status: util.IntPtrInt(db.NETWORK_ACTIVE),
					},
				}
				err = v.UpdateVirtualNetworkById(netSpec.Id, net)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Keyからネットワーク情報を取得", func() {
				net, err := v.GetVirtualNetworkById(netSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				jsonData, err := json.MarshalIndent(net, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(jsonData))
				Expect(*net.Status.Status).To(Equal(db.NETWORK_ACTIVE))
			})

			It("ネットワークの作成 #2", func() {
				net := &api.VirtualNetwork{
					Metadata: &api.Metadata{
						Name: util.StringPtr("net02"),
					},
					Spec: &api.VlanSpec{
						BridgeName:       util.StringPtr("br01"),
						DhcpEndAddress:   util.StringPtr("192.168.122.0"),
						DhcpStartAddress: util.StringPtr("192.168.122.254"),
						MacAddress:       util.StringPtr("52:54:00:6b:3c:58"),
						ForwardMode:      util.StringPtr("NAT"),
					},
				}
				netSpec, err := v.CreateVirtualNetwork(*net)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created network with ID:", netSpec.Id)
			})

			It("ネットワークの作成 #3", func() {
				net := &api.VirtualNetwork{
					Metadata: &api.Metadata{
						Name: util.StringPtr("net03"),
					},
					Spec: &api.VlanSpec{
						BridgeName:       util.StringPtr("br01"),
						DhcpEndAddress:   util.StringPtr("192.168.122.0"),
						DhcpStartAddress: util.StringPtr("192.168.122.254"),
						MacAddress:       util.StringPtr("52:54:00:6b:3c:58"),
						ForwardMode:      util.StringPtr("NAT"),
					},
				}
				netSpec, err := v.CreateVirtualNetwork(*net)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created network with ID:", netSpec.Id)
			})

			It("ネットワークの一覧取得", func() {
				nets, err = v.GetVirtualNetworks()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nets)).To(Equal(3))
				fmt.Println("ネットワーク一覧:")
				for _, net := range nets {
					jsonData, err := json.MarshalIndent(net, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})

			It("ネットワークの削除", func() {
				err = v.DeleteVirtualNetworkById(nets[0].Id)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ネットワークの一覧取得", func() {
				nets, err = v.GetVirtualNetworks()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nets)).To(Equal(2))
				fmt.Println("ネットワーク一覧:")
				for _, net := range nets {
					jsonData, err := json.MarshalIndent(net, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})
		})
	})
})
