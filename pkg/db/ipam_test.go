package db_test

import (
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
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("IPAM", Ordered, func() {
	var url string = "http://127.0.0.1:9379"
	var containerID string
	var idIpv4_1, idIpv4_2, idIpv6_1 string

	BeforeAll(func(ctx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "jobEtcdDb", "-p", "9379:2379", "-p", "9380:2380", "ghcr.io/takara9/etcd:3.6.5")
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

	Describe("IPアドレス管理テスト", Ordered, func() {
		var v *db.Database
		var id string
		var nets []api.IPNetwork
		Context("IPv4のテスト", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ネットワークアドレスの作成 #1", func() {
				net := &api.IPNetwork{
					AddressMaskLen: util.StringPtr("192.168.200.0/24"),
				}
				id, err = v.CreateIpNetwork(net)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created network with ID:", id)
				idIpv4_1 = id
			})

			It("Keyからネットワーク情報を取得", func() {
				GinkgoWriter.Printf("ネットワークID: %s\n", id)
				net, err := v.GetIpNetworkById(id)
				Expect(err).NotTo(HaveOccurred())
				bytes, err := json.MarshalIndent(net, "", "    ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(bytes))
			})

			It("ネットワークアドレスの作成 #2", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("192.168.200.0/24"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				Expect(err).To(HaveOccurred())
			})

			It("ネットワークアドレスの作成 #3", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("192.168.200.0/25"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				Expect(err).To(HaveOccurred())
			})

			It("ネットワークアドレスの作成 #4", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("192.168.201.0/24"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				idIpv4_2 = id
				Expect(err).ToNot(HaveOccurred())
			})

			It("ネットワークの一覧取得", func() {
				nets, err = v.GetIpNetworks()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nets)).To(Equal(2))
				fmt.Println("ネットワーク一覧:")
				for _, net := range nets {
					jsonData, err := json.MarshalIndent(net, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})

			//It("ネットワークの削除", func() {
			//	err = v.DeleteIpNetworkById(nets[0].Id)
			//	Expect(err).NotTo(HaveOccurred())
			//})

			It("ネットワークの一覧取得", func() {
				nets, err = v.GetIpNetworks()
				Expect(err).NotTo(HaveOccurred())
				//Expect(len(nets)).To(Equal(1))
				fmt.Println("ネットワーク一覧:")
				for _, net := range nets {
					jsonData, err := json.MarshalIndent(net, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})

		})
		Context("IPv6のテスト", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ネットワークアドレスの作成 ULA #1", func() {
				net := &api.IPNetwork{
					AddressMaskLen: util.StringPtr("fd00::/64"),
				}
				id, err = v.CreateIpNetwork(net)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created network with ID:", id)
				idIpv6_1 = id
			})

			It("Keyからネットワーク情報を取得", func() {
				GinkgoWriter.Printf("ネットワークID: %s\n", id)
				net, err := v.GetIpNetworkById(id)
				Expect(err).NotTo(HaveOccurred())
				bytes, err := json.MarshalIndent(net, "", "    ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(bytes))
			})

			It("ネットワークアドレスの作成 同じULA を表現を変えて設定 #2", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("fd00::0:0:0:0/64"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				Expect(err).To(HaveOccurred())
			})

			It("ネットワークアドレスの作成 同じULA を表現を変えて設定 #3", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("fd00::1/64"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				Expect(err).To(HaveOccurred())
			})

			It("ネットワークアドレスの作成 サブネットを変えて設定 #4", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("fd00::1:0:0:0:0/64"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				Expect(err).ToNot(HaveOccurred())
			})

			It("ネットワークアドレスの作成 サブネットを変えて設定 #5", func() {
				net := &api.IPNetwork{
					Id:             "",
					AddressMaskLen: util.StringPtr("fd00::2:0:0:0:0/64"),
				}
				id, err = v.CreateIpNetwork(net)
				fmt.Println("Created network with ID:", id)
				Expect(err).ToNot(HaveOccurred())
			})

			It("ネットワークの一覧取得", func() {
				nets, err = v.GetIpNetworks()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("ネットワーク一覧:")
				for i, net := range nets {
					fmt.Println(i, net.Id, *net.AddressMaskLen)
				}
			})

			//It("ネットワークの削除", func() {
			//	err = v.DeleteIpNetworkById(nets[0].Id)
			//	Expect(err).NotTo(HaveOccurred())
			//})

			It("ネットワークの一覧取得", func() {
				nets, err = v.GetIpNetworks()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("ネットワーク一覧:")
				for i, net := range nets {
					fmt.Println(i, net.Id, *net.AddressMaskLen)
				}
			})
		})

		Context("IPv4アドレスの振り出し", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			var ipaddr string
			It("IPアドレスの割り当て #1", func() {
				fmt.Println("==== ネットワークID for IPv4:", idIpv4_1)

				_, err := v.GetIpNetworkById(idIpv4_1)
				Expect(err).NotTo(HaveOccurred())
				ip, err := v.AllocateIP(idIpv4_1, "host1")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
				ipaddr = ip
			})

			It("IPアドレスのリリース #1", func() {
				err = v.ReleaseIP(idIpv4_1, ipaddr)
				Expect(err).NotTo(HaveOccurred())
			})

			It("IPアドレスの割り当て #2", func() {
				_, err := v.GetIpNetworkById(idIpv4_1)
				Expect(err).NotTo(HaveOccurred())
				ip, err := v.AllocateIP(idIpv4_1, "host2")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割り当て #3", func() {
				_, err := v.GetIpNetworkById(idIpv4_1)
				Expect(err).NotTo(HaveOccurred())
				ip, err := v.AllocateIP(idIpv4_1, "host3")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			// IPアドレスだけで消して、これで良いのか？
			// 通常は、ホスト名も消すべきではないか？　ホスト名を消すと、IPアドレスの再利用ができなくなるのではないか？
			It("IPアドレスのリリース #1", func() {
				err = v.ReleaseIP(idIpv4_1, ipaddr)
				Expect(err).NotTo(HaveOccurred())
			})

			It("IPアドレスの割り当て #2", func() {
				for i := 2; i < 254; i++ {
					hostname := fmt.Sprintf("host%d", i)
					ip, err := v.AllocateIP(idIpv4_1, hostname)
					Expect(err).NotTo(HaveOccurred())
					fmt.Printf("Allocated IP: %s\n", ip)
				}
			})

			It("IPアドレスの割り当て #3", func() {
				ip, err := v.AllocateIP(idIpv4_1, "host255")
				Expect(err).To(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割り当て #4", func() {
				ip, err := v.AllocateIP(idIpv4_2, "host_b01")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割り当て #5", func() {
				ip, err := v.AllocateIP(idIpv4_2, "host_b02")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割り当て #5", func() {
				ip, err := v.AllocateIP(idIpv4_2, "host_b03")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割当リストの取得", func() {
				ips, err := v.GetAllocatedIPs(idIpv4_1)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IPs for network %s:\n", idIpv4_1)
				for _, ip := range ips {
					fmt.Printf("- Host: %s  IP: %s\n", *ip.HostId, *ip.IPAddress)
				}
			})

			It("データベース接続のクローズ", func() {
				Expect(v.Close()).NotTo(HaveOccurred())
			})

			//It("データベース接続のクローズ後に操作を試みる", func() {
			//	_, err := v.GetIpNetworkById(idIpv4)
			//	Expect(err).To(HaveOccurred())
			//})
		})

		Context("IPv6アドレスの振り出し", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("IPアドレスの割り当て #1", func() {
				_, err := v.GetIpNetworkById(idIpv6_1)
				Expect(err).NotTo(HaveOccurred())
				ip, err := v.AllocateIP(idIpv6_1, "host1")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割り当て #2", func() {
				for i := 2; i < 20; i++ {
					hostname := fmt.Sprintf("host%d", i)
					ip, err := v.AllocateIP(idIpv6_1, hostname)
					Expect(err).NotTo(HaveOccurred())
					fmt.Printf("Allocated IP: %s\n", ip)
				}
			})

			It("IPアドレスの割り当て #3", func() {
				ip, err := v.AllocateIP(idIpv6_1, "host255")
				Expect(err).ToNot(HaveOccurred())
				fmt.Printf("Allocated IP: %s\n", ip)
			})

			It("IPアドレスの割当リストの取得", func() {
				ips, err := v.GetAllocatedIPs(idIpv6_1)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Allocated IPs for network %s:\n", idIpv6_1)
				for _, ip := range ips {
					fmt.Printf("- Host: %s  IP: %s\n", *ip.HostId, *ip.IPAddress)
				}
			})

			It("データベース接続のクローズ", func() {
				Expect(v.Close()).NotTo(HaveOccurred())
			})

			//	It("データベース接続のクローズ後に操作を試みる", func() {
			//		_, err := v.GetIpNetworkById(idIpv6)
			//		Expect(err).To(HaveOccurred())
			//	})

		})

		Context("ネットワークの削除", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ネットワークの削除", func() {
				err = v.DeleteIpNetworkById(idIpv4_1)
				Expect(err).To(HaveOccurred()) // 割り当てられたIPがあるため、削除できないことを期待

				err = v.DeleteIpNetworkById(idIpv4_2)
				Expect(err).To(HaveOccurred()) // 割り当てられたIPがあるため、削除できないことを期待

				err = v.DeleteIpNetworkById(idIpv6_1)
				Expect(err).To(HaveOccurred()) // 割り当てられたIPがあるため、削除できないことを期待
			})

			It("IPアドレスのリリース #1", func() {
				ips, err := v.GetAllocatedIPs(idIpv4_1)
				Expect(err).NotTo(HaveOccurred())
				for _, ip := range ips {
					err = v.ReleaseIP(idIpv4_1, *ip.IPAddress)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("IPアドレスのリリース #2", func() {
				ips, err := v.GetAllocatedIPs(idIpv4_2)
				Expect(err).NotTo(HaveOccurred())
				for _, ip := range ips {
					err = v.ReleaseIP(idIpv4_2, *ip.IPAddress)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("IPアドレスのリリース #3", func() {
				ips, err := v.GetAllocatedIPs(idIpv6_1)
				Expect(err).NotTo(HaveOccurred())
				for _, ip := range ips {
					err = v.ReleaseIP(idIpv6_1, *ip.IPAddress)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("ネットワークの削除", func() {
				err = v.DeleteIpNetworkById(idIpv4_1)
				Expect(err).NotTo(HaveOccurred())
				_, err = v.GetIpNetworkById(idIpv4_1)
				Expect(err).To(HaveOccurred()) // 削除されたネットワークは取得できないことを期待

				err = v.DeleteIpNetworkById(idIpv4_2)
				Expect(err).NotTo(HaveOccurred())
				_, err = v.GetIpNetworkById(idIpv4_2)
				Expect(err).To(HaveOccurred()) // 削除されたネットワークは取得できないことを期待

				err = v.DeleteIpNetworkById(idIpv6_1)
				Expect(err).NotTo(HaveOccurred())
				_, err = v.GetIpNetworkById(idIpv6_1)
				Expect(err).To(HaveOccurred()) // 削除されたネットワークは取得できないことを期待
			})

			It("データベース接続のクローズ", func() {
				Expect(v.Close()).NotTo(HaveOccurred())
			})
		})
	})
})
