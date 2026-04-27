package main

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
		})
	})

	Context("様々に条件を変えてサーバーのデプロイ", func() {
		var serverId_1, serverId_2, serverId_3 string

		It("外部接続の仮想サーバー作成 test-23", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-23-test-net-3-host-bridge-ip.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("外部接続の仮想サーバーの状態確認 test-23", func() {
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

		It("外部接続の仮想サーバーの削除 test-23", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("外部接続の仮想サーバーの削除状態確認 test-23", func() {
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

		It("外部接続の仮想サーバーで削除を確認 test-23", func() {
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

		// マルチホームの仮想サーバー作成 test-28,29,30
		It("マルチホームの仮想サーバー作成 test-28,29,30", func() {
			for i := 28; i <= 30; i++ {
				configFile := fmt.Sprintf("testdata/test-server-%02d-multihome.yaml", i)
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", configFile, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				switch i {
				case 28:
					serverId_1 = server.Id
				case 29:
					serverId_2 = server.Id
				case 30:
					serverId_3 = server.Id
				}
			}
		})

		It("マルチホームの仮想サーバーの状態確認 test-28", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("マルチホームの仮想サーバーの状態確認 test-29", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_2, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err := json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("マルチホームの仮想サーバーの状態確認 test-30", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", serverId_3, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err := json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", *server.Metadata.Name, server.Id)
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("マルチホームの仮想サーバーの削除 test-28,29,30", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))
			}
		})

		It("マルチホームの仮想サーバーの削除状態確認 test-28,29,30", func() {
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

		It("マルチホームの仮想サーバーで削除を確認 test-28,29,30", func() {
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

		// マルチホームでホストブリッジ接続の仮想サーバーのテスト test-31,32,33
		It("マルチホームでホストブリッジ接続の仮想サーバー作成 test-31,32,33", func() {
			for i := 31; i <= 33; i++ {
				configFile := fmt.Sprintf("testdata/test-server-%02d-multihome-host-bridge.yaml", i)
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", configFile, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				switch i {
				case 31:
					serverId_1 = server.Id
				case 32:
					serverId_2 = server.Id
				case 33:
					serverId_3 = server.Id
				}
			}
		})

		It("マルチホームでホストブリッジ接続の仮想サーバーの状態確認 test-31,32,33", func() {
			for i := 31; i <= 33; i++ {
				Eventually(func(g Gomega) {
					serverId := ""
					switch i {
					case 31:
						serverId = serverId_1
					case 32:
						serverId = serverId_2
					case 33:
						serverId = serverId_3
					}
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

		It("マルチホームでホストブリッジ接続の仮想サーバーの削除 test-31,32,33", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))
			}
		})

		It("マルチホームでホストブリッジ接続の仮想サーバーの削除状態確認 test-31,32,33", func() {
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

		It("マルチホームでホストブリッジ接続の仮想サーバーで削除を確認 test-31,32,33", func() {
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

		It("SSH-KEYを設定したサーバー作成 test-34", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-34-sshkey.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("外部接続の仮想サーバーの状態確認 test-34", func() {
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

		It("外部接続の仮想サーバーの削除 test-34", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("外部接続の仮想サーバーの削除状態確認 test-34", func() {
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

		It("外部接続の仮想サーバーで削除を確認 test-34", func() {
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
