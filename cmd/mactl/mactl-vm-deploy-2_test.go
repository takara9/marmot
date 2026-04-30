package main

import (
	"encoding/json"
	"errors"
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

var _ = Describe("MarmotdTest", Ordered, func() {
	var mockServer *mockServerHandle
	var containerID string

	BeforeAll(func(specCtx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			//Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
		cleanupTestEnvironment()

		By("モックサーバー用etcdの起動")
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", "3379:2379", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)

		By("モックサーバーの起動")
		mockServer, err = startMockServer()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func(specCtx SpecContext) {
		mockServer.Stop() // モックサーバー停止
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
		os.Remove("bin/mactl-test")
		os.Remove("/var/actions-runner/_work/marmot/marmot/cmd/mactl/bin/mactl-test")
		cleanupTestEnvironment()

		By("ネットワークの削除 デファルト")
		cmd = exec.Command("virsh", "net-destroy", "default")
		stdoutStderr, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
		cmd = exec.Command("virsh", "net-undefine", "default")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))

		By("ネットワークの削除 ホストブリッジ")
		cmd = exec.Command("virsh", "net-destroy", "host-bridge")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
		cmd = exec.Command("virsh", "net-undefine", "host-bridge")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))

		By("ネットワークの削除 ovs-network")
		cmd = exec.Command("virsh", "net-destroy", "ovs-network")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
		cmd = exec.Command("virsh", "net-undefine", "ovs-network")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))

		By("ネットワークの削除 test-net-1")
		cmd = exec.Command("virsh", "net-destroy", "test-net-1")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
		cmd = exec.Command("virsh", "net-undefine", "test-net-1")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))

		By("ネットワークの削除 test-net-2")
		cmd = exec.Command("virsh", "net-destroy", "test-net-2")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
		cmd = exec.Command("virsh", "net-undefine", "test-net-2")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))

		By("ネットワークの削除 test-net-3")
		cmd = exec.Command("virsh", "net-destroy", "test-net-3")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
		cmd = exec.Command("virsh", "net-undefine", "test-net-3")
		stdoutStderr, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Println(string(stdoutStderr))
	})

	Context("クライアントからアクセステスト", func() {
		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8080/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("基礎ネットワークの準備", func() {
		var deletePropagationHeadID string
		var deletePropagationFollowerID string

		It("定義 default", func() {
			By("定義設定 デファルト")
			cmd := exec.Command("virsh", "net-define", "testdata/default-network.xml")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())

			By("開始 デファルト")
			cmd = exec.Command("virsh", "net-start", "default")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())

			By("自動起動設定  デファルト")
			cmd = exec.Command("virsh", "net-autostart", "default")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("定義 host-bridge", func() {
			By("定義設定 ホストブリッジ")
			cmd := exec.Command("virsh", "net-define", "testdata/host-bridge.xml")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())

			By("開始 ホストブリッジ")
			cmd = exec.Command("virsh", "net-start", "host-bridge")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())

			By("自動起動設定  ホストブリッジ")
			cmd = exec.Command("virsh", "net-autostart", "host-bridge")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("ネットワークの定義 ovs-network", func() {
			By("定義設定Open-VSwitch")
			cmd := exec.Command("virsh", "net-define", "testdata/ovs-network.xml")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())

			By("開始 Open-VSwitch")
			cmd = exec.Command("virsh", "net-start", "ovs-network")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())

			By("自動起動設定  Open-VSwitch")
			cmd = exec.Command("virsh", "net-autostart", "ovs-network")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			Expect(err).NotTo(HaveOccurred())
		})

		It("DB登録をチェック", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var networks []api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &networks)
				g.Expect(err).NotTo(HaveOccurred())
				for _, n := range networks {
					GinkgoWriter.Printf("  - %s (%s)\n", *n.Metadata.Name, n.Id)
					g.Expect(n.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
				}
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("仮想ネットワークのリスト", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		var netId1 string
		It("仮想ネットワークの作成 test-net-1", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "create", "--configfile", "testdata/test-network-01-test-net-1.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var net api.VirtualNetwork
			err = json.Unmarshal(stdoutStderr, &net)
			Expect(err).NotTo(HaveOccurred())
			Expect(*net.Metadata.Name).To(Equal("test-net-1"))
			netId1 = net.Id
		})
		fmt.Printf("netId1: %s\n", netId1)

		It("仮想ネットワークの状態確認 test-net-1", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "detail", netId1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var network api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &network)
				g.Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *network.Metadata.Name, network.Id)
				g.Expect(network.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		var netId2 string
		It("仮想ネットワークの作成 test-net-2 raw URL", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "create", "--configfile", testNetworkConfigRawURL, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var net api.VirtualNetwork
			err = json.Unmarshal(stdoutStderr, &net)
			Expect(err).NotTo(HaveOccurred())
			Expect(*net.Metadata.Name).To(Equal("test-net-2"))
			netId2 = net.Id
		})
		fmt.Printf("netId2: %s\n", netId2)

		It("仮想ネットワークの状態確認 test-net-2", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "detail", netId2, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var network api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &network)
				g.Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *network.Metadata.Name, network.Id)
				g.Expect(network.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		var netId3 string
		It("仮想ネットワークの作成 test-net-3", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "create", "--configfile", "testdata/test-network-03-host-bridge.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var net api.VirtualNetwork
			err = json.Unmarshal(stdoutStderr, &net)
			Expect(err).NotTo(HaveOccurred())
			Expect(*net.Metadata.Name).To(Equal("test-net-3"))
			netId3 = net.Id
		})
		fmt.Printf("netId3: %s\n", netId3)

		It("仮想ネットワークの状態確認 test-net-3", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "detail", netId3, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var network api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &network)
				g.Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *network.Metadata.Name, network.Id)
				g.Expect(network.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("削除伝播検証用に同名ネットワークオブジェクトを準備", func() {
			labels := map[string]interface{}{}
			db.SetNetworkSyncLabels(labels, "head", "", "hvc")

			headSpec := api.VirtualNetwork{
				Metadata: &api.Metadata{
					Name:     util.StringPtr("delete-propagation-net"),
					NodeName: util.StringPtr("hvc"),
					Labels:   &labels,
				},
				Spec: &api.VirtualNetworkSpec{
					BridgeName: util.StringPtr("default"),
				},
			}

			head, err := mockServer.server.Ma.Db.CreateVirtualNetwork(headSpec)
			Expect(err).NotTo(HaveOccurred())
			deletePropagationHeadID = head.Id
			mockServer.server.Ma.Db.UpdateVirtualNetworkStatus(deletePropagationHeadID, db.NETWORK_ACTIVE)

			headFromDB, err := mockServer.server.Ma.Db.GetVirtualNetworkById(deletePropagationHeadID)
			Expect(err).NotTo(HaveOccurred())

			deletePropagationFollowerID, err = mockServer.server.Ma.Db.MakeFollowerVirtualNetworkEntry(headFromDB, "hvc", deletePropagationHeadID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deletePropagationFollowerID).NotTo(Equal(deletePropagationHeadID))

			follower, err := mockServer.server.Ma.Db.GetVirtualNetworkById(deletePropagationFollowerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(follower.Metadata).NotTo(BeNil())
			Expect(follower.Metadata.Name).NotTo(BeNil())
			Expect(*follower.Metadata.Name).To(Equal("delete-propagation-net"))
			Expect(follower.Status).NotTo(BeNil())
			Expect(follower.Status.DeletionTimeStamp).To(BeNil())
		})

		It("mactl network delete実行で同名オブジェクトにDeletionTimeStampが伝播する", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "delete", deletePropagationHeadID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			Eventually(func(g Gomega) {
				head, err := mockServer.server.Ma.Db.GetVirtualNetworkById(deletePropagationHeadID)
				if err == nil {
					g.Expect(head.Status).NotTo(BeNil())
					g.Expect(head.Status.DeletionTimeStamp).NotTo(BeNil())
				} else {
					// 削除処理が進んでいる場合は、DBエントリーが先に消えることがある。
					g.Expect(errors.Is(err, db.ErrNotFound)).To(BeTrue())
				}

				follower, err := mockServer.server.Ma.Db.GetVirtualNetworkById(deletePropagationFollowerID)
				if err == nil {
					g.Expect(follower.Status).NotTo(BeNil())
					g.Expect(follower.Status.DeletionTimeStamp).NotTo(BeNil())
				} else {
					g.Expect(errors.Is(err, db.ErrNotFound)).To(BeTrue())
				}
			}, 8*time.Second, 1*time.Second).Should(Succeed())
		})

		It("仮想ネットワークのリスト", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("IPネットワークのリスト ipn", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "ipn", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var ipnets []api.IPNetwork
			err = json.Unmarshal(stdoutStderr, &ipnets)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(ipnets)).To(BeNumerically(">", 0))
		})

		It("仮想ネットワーク配下のIPネットワークのリスト ipn-by-vn", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "ipn-by-vn", netId1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var ipnets []api.IPNetwork
			err = json.Unmarshal(stdoutStderr, &ipnets)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(ipnets)).To(BeNumerically(">", 0))
		})
	})

	Context("OSイメージの準備", func() {
		It("OSイメージのリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "image", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
			var images []api.Image
			err = json.Unmarshal(stdoutStderr, &images)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(images)).To(Equal(0))
		})

		var imageID string
		It("OSイメージの登録", func() {
			url := "http://hmc/ubuntu-22.04-server-cloudimg-amd64.img"
			imageName := "ubuntu22.04"
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "image", "create", imageName, url, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
			var image api.Success
			err = json.Unmarshal(stdoutStderr, &image)
			Expect(err).NotTo(HaveOccurred())
			imageID = image.Id
		})

		It("OSイメージの個別詳細取得", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "image", "detail", imageID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
				var image api.Image
				err = json.Unmarshal(stdoutStderr, &image)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(image.Status.StatusCode).To(Equal(db.IMAGE_AVAILABLE))
			}, 10*60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("OSイメージのリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "image", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
			assertImageListTextHeader(stdoutStderr)
		})
	})

	Context("様々に条件を変えてサーバーのデプロイ", func() {
		var serverId_1, serverId_2, serverId_3 string
		It("仮想サーバー作成 test-11 仮想ネットへ接続の仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-11-net-2-auto-IP.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバー作成 test-12 仮想ネットへ接続の仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-12-net-2-auto-IP.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_2 = server.Id
		})

		It("仮想サーバーの状態確認 test-11", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("仮想サーバーの状態確認 test-12", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_2, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("仮想サーバーのリスト表示 test-11, test-12", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除 test-11", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除 test-12", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_2, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-11", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("仮想サーバーの削除状態確認 test-12", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_2, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("仮想サーバーで削除を確認 test-11, test-12", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			By("仮想サーバーのリスト JSON形式")
			Eventually(func(g Gomega) {
				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var servers []api.Server
				err = json.Unmarshal(stdoutStderr, &servers)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(servers)).To(Equal(0))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("仮想サーバー作成 test-20,21,22 同一仮想ネットで複数の仮想マシン", func() {
			for i := 20; i <= 22; i++ {
				configFile := fmt.Sprintf("testdata/test-server-%02d-test-net-3.yaml", i)
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", configFile, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
				switch i {
				case 20:
					serverId_1 = server.Id
				case 21:
					serverId_2 = server.Id
				case 22:
					serverId_3 = server.Id
				}
			}
		})

		It("仮想サーバーの状態確認 test-20,21,22", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				Eventually(func(g Gomega) {
					cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId, "--output", "json")
					stdoutStderr, err := cmd.CombinedOutput()
					g.Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(stdoutStderr))

					var server api.Server
					err = json.Unmarshal(stdoutStderr, &server)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
					g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
					expectServerBootVolumeNodeName(g, server)
				}, 120*time.Second, 5*time.Second).Should(Succeed())
			}
		})

		It("仮想サーバーをリストで確認 test-20, test-21, test-22", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除 test-20,21,22", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))
			}
		})

		It("仮想サーバーの削除状態確認 test-20,21,22", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				Eventually(func(g Gomega) {
					cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId, "--output", "json")
					stdoutStderr, err := cmd.CombinedOutput()
					g.Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(stdoutStderr))

					var server api.Server
					err = json.Unmarshal(stdoutStderr, &server)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
					g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
				}, 120*time.Second, 5*time.Second).Should(Succeed())
			}
		})

		It("仮想サーバーで削除を確認 test-20,21,22", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			By("仮想サーバーのリスト JSON形式")
			Eventually(func(g Gomega) {
				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var servers []api.Server
				err = json.Unmarshal(stdoutStderr, &servers)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(servers)).To(Equal(0))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})
	})
})
