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

const testNetworkConfigRawURL = "https://raw.githubusercontent.com/takara9/marmot/refs/heads/main/cmd/mactl/testdata/test-network-02-test-net-2.yaml"

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
			//Expect(err).NotTo(HaveOccurred())

			By("開始 デファルト")
			cmd = exec.Command("virsh", "net-start", "default")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())

			By("自動起動設定  デファルト")
			cmd = exec.Command("virsh", "net-autostart", "default")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())
		})

		It("定義 host-bridge", func() {
			By("定義設定 ホストブリッジ")
			cmd := exec.Command("virsh", "net-define", "testdata/host-bridge.xml")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())

			By("開始 ホストブリッジ")
			cmd = exec.Command("virsh", "net-start", "host-bridge")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())

			By("自動起動設定  ホストブリッジ")
			cmd = exec.Command("virsh", "net-autostart", "host-bridge")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())
		})

		It("ネットワークの定義 ovs-network", func() {
			By("定義設定Open-VSwitch")
			cmd := exec.Command("virsh", "net-define", "testdata/ovs-network.xml")
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())

			By("開始 Open-VSwitch")
			cmd = exec.Command("virsh", "net-start", "ovs-network")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())

			By("自動起動設定  Open-VSwitch")
			cmd = exec.Command("virsh", "net-autostart", "ovs-network")
			stdoutStderr, err = cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Println(string(stdoutStderr))
			}
			//Expect(err).NotTo(HaveOccurred())
		})

		It("DB登録をチェック JSON形式", func() {
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

		It("仮想ネットワークのリスト テキスト形式", func() {
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
			assertImageListTextHeader(stdoutStderr)
		})
	})

	Context("様々に条件を変えてサーバーのデプロイ", func() {
		var serverId_1, serverId_2 string
		It("仮想サーバー作成 test-00", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-00-none.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})
		It("仮想サーバーの状態確認 test-00", func() {
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

		It("仮想サーバーの削除 test-00", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-00", func() {
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

		It("仮想サーバーのリスト", func() {
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

		It("仮想サーバー作成 test-01 defaultネットワークに繋いだ", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-01-default.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-01", func() {
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

		It("仮想サーバーの削除 test-01", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-01", func() {
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

		It("仮想サーバーのリスト", func() {
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

		// ホストが繋がっているネットワークに繋がったて、DHCPを利用する仮想マシンを作成する
		It("仮想サーバー作成 test-02 host-bridgeネットワークに繋いだ", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-02-host-bridge.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-02", func() {
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

		It("仮想サーバーの削除 test-02", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-02", func() {
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

		It("仮想サーバーで削除を確認 test-02", func() {
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

		// ホストのネットワークに繋がった仮想マシンで、IPアドレス指定で起動する
		It("仮想サーバー作成 test-03 host-bridgeでIPアドレス指定", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-03-host-bridge-ip.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-03", func() {
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

		It("仮想サーバーの削除 test-03", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-03", func() {
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

		It("仮想サーバーで削除を確認 test-03", func() {
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

		// 複数ボリュームを待つ仮想マシンを作成する
		It("仮想サーバー作成 test-04 複数ボリューム", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-04-multi-vol-qcow2.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-04", func() {
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

		It("仮想サーバーの削除 test-04", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-04", func() {
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

		It("仮想サーバーで削除を確認 test-04", func() {
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

		// LVMのブートボリュームで起動する仮想マシンを作成
		It("仮想サーバー作成 test-05 +LVMボリュームで起動する仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-05-boot-lvm.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-05", func() {
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

		It("仮想サーバーの削除 test-05", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-05", func() {
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

		It("仮想サーバーで削除を確認 test-05", func() {
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

		// LVMでブートするマシンで複数ボリュームを待つ仮想マシンを作成する
		It("仮想サーバー作成 test-06 KVMボリュームで起動、複数のボリュームを持つ仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-06-multivol-lvm.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-06", func() {
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

		It("仮想サーバーの削除 test-06", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-06", func() {
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

		It("仮想サーバーで削除を確認 test-06", func() {
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

		//host-bridgeネットワークに繋がった仮想マシンで、IPアドレス指定で起動する。
		It("仮想サーバー作成 test-07 KVMボリュームで起動、複数のボリュームを持つ仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-07-host-bridge-ip.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-07", func() {
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

		It("仮想サーバーの削除 test-07", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-07", func() {
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

		It("仮想サーバーで削除を確認 test-07", func() {
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

		// ovs-networkの存在チェック
		It("ovs-networkの存在確認", func() {
			cmd := exec.Command("virsh", "net-list")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
			Expect(string(stdoutStderr)).To(ContainSubstring("ovs-network"))
		})

		It("ovs-networkの存在確認-2", func() {
			cmd := exec.Command("virsh", "net-dumpxml", "ovs-network")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
			Expect(string(stdoutStderr)).To(ContainSubstring("ovsbr0"))
		})

		//VLANに接続する仮想マシンで、IPアドレス指定で起動する。
		It("仮想サーバー作成 test-08 VLAN接続の仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-08-host-bridge-vlan.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-08", func() {
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

		It("仮想サーバーの削除 test-08", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-08", func() {
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

		It("仮想サーバーで削除を確認 test-08", func() {
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

		//仮想ネットへ接続の仮想マシン 09
		It("仮想サーバー作成 test-09 仮想ネットへ接続の仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-09-net-2-set-IP.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_1 = server.Id
		})

		It("仮想サーバーの状態確認 test-09", func() {
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

		It("仮想サーバー作成 test-10 仮想ネットへ接続の仮想マシン", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--configfile", "testdata/test-server-10-net-2-set-IP.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var server api.Server
			err = json.Unmarshal(stdoutStderr, &server)
			Expect(err).NotTo(HaveOccurred())
			//Expect(*server.Metadata.Name).To(Equal("test-server-00"))
			serverId_2 = server.Id
		})

		It("仮想サーバーの状態確認 test-10", func() {
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

		It("仮想サーバーのリスト表示 test-9, test-10", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("割り当て済みIPアドレスのリスト ips", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())

				var vnets []api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &vnets)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vnets)).To(BeNumerically(">", 0))

				var defaultVnetID string
				for _, v := range vnets {
					if v.Metadata != nil && v.Metadata.Name != nil && *v.Metadata.Name == "test-net-2" {
						defaultVnetID = v.Id
						break
					}
				}
				g.Expect(defaultVnetID).NotTo(BeEmpty())

				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "ipn-by-vn", defaultVnetID, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())

				var ipnets []api.IPNetwork
				err = json.Unmarshal(stdoutStderr, &ipnets)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(ipnets)).To(BeNumerically(">", 0))

				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "network", "ips", ipnets[0].Id, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var addrs []api.IPAddress
				err = json.Unmarshal(stdoutStderr, &addrs)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(addrs)).To(BeNumerically(">", 0))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("仮想サーバーの削除 test-09", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-09", func() {
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

		It("仮想サーバーの削除 test-10", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", serverId_2, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("仮想サーバーの削除状態確認 test-10", func() {
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

		It("仮想サーバーで削除を確認 test-09, test-10", func() {
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
