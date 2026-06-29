package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("MarmotdTest", Ordered, func() {
	var mockServer *mockServerHandle
	var containerID string
	var serverId_1, serverId_2, serverId_3 string
	var testHomeDir string

	BeforeAll(func(specCtx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			//Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
		cleanupTestEnvironment()
		var err error
		testHomeDir, err = setupMactlTestHome()
		Expect(err).NotTo(HaveOccurred())
		if err := ensureMactlTestBinary(); err != nil {
			Fail(fmt.Sprintf("Failed to build mactl test binary: %v", err))
		}

		By("モックサーバー用etcdの起動")
		var etcdEp string
		containerID, etcdEp, err = startEtcdContainer()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %v", err))
		}
		fmt.Printf("Container started with ID: %s\n", containerID)

		By("モックサーバーの起動")
		mockServer, err = startMockServer(etcdEp)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginAsAdmin()).NotTo(HaveOccurred())
	})

	AfterAll(func(specCtx SpecContext) {
		// マルチホームの仮想サーバーの削除
		for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "delete", serverId, "--output", "json")
			stdoutStderr, _ := cmd.CombinedOutput()
			fmt.Println(string(stdoutStderr))
		}

		// マルチホームの仮想サーバーの削除状態確認
		for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
			if strings.TrimSpace(serverId) == "" {
				continue
			}
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId, "--output", "json")
				stdoutStderr, _ := cmd.CombinedOutput()
				output := strings.ToLower(string(stdoutStderr))
				fmt.Println("checking not found:", string(stdoutStderr))
				g.Expect(
					strings.Contains(output, "not found") ||
						strings.Contains(output, "404") ||
						strings.Contains(output, "idが存在しません"),
				).To(BeTrue())
			}, 60*time.Second, 5*time.Second).Should(Succeed())
		}

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
		if strings.TrimSpace(testHomeDir) != "" {
			_ = os.RemoveAll(testHomeDir)
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var networks []api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &networks)
				Expect(err).NotTo(HaveOccurred())
				for _, n := range networks {
					GinkgoWriter.Printf("  - %s (%s)\n", n.Metadata.Name, api.VirtualNetworkID(n))
					g.Expect(n.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
				}
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("仮想ネットワークのリスト", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		var netId1 string
		It("仮想ネットワークの作成 test-net-1", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "create", "--configfile", "testdata/test-network-01-test-net-1.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var net api.VirtualNetwork
			err = json.Unmarshal(stdoutStderr, &net)
			Expect(err).NotTo(HaveOccurred())
			Expect(net.Metadata.Name).To(Equal("test-net-1"))
			netId1 = api.VirtualNetworkID(net)
		})
		fmt.Printf("netId1: %s\n", netId1)

		It("仮想ネットワークの状態確認 test-net-1", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "detail", netId1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var network api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &network)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", network.Metadata.Name, api.VirtualNetworkID(network))
				g.Expect(network.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		var netId2 string
		It("仮想ネットワークの作成 test-net-2 raw URL", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "create", "--configfile", "testdata/test-network-02-test-net-2.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var net api.VirtualNetwork
			err = json.Unmarshal(stdoutStderr, &net)
			Expect(err).NotTo(HaveOccurred())
			Expect(net.Metadata.Name).To(Equal("test-net-2"))
			netId2 = api.VirtualNetworkID(net)
		})
		fmt.Printf("netId2: %s\n", netId2)

		It("仮想ネットワークの状態確認 test-net-2", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "detail", netId2, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var network api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &network)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", network.Metadata.Name, api.VirtualNetworkID(network))
				g.Expect(network.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		var netId3 string
		It("仮想ネットワークの作成 test-net-3", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "create", "--configfile", "testdata/test-network-03-host-bridge.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var net api.VirtualNetwork
			err = json.Unmarshal(stdoutStderr, &net)
			Expect(err).NotTo(HaveOccurred())
			Expect(net.Metadata.Name).To(Equal("test-net-3"))
			netId3 = api.VirtualNetworkID(net)
		})
		fmt.Printf("netId3: %s\n", netId3)

		It("仮想ネットワークの状態確認 test-net-3", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "detail", netId3, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var network api.VirtualNetwork
				err = json.Unmarshal(stdoutStderr, &network)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("  - %s (%s)\n", network.Metadata.Name, api.VirtualNetworkID(network))
				g.Expect(network.Status.StatusCode).To(Equal(int(db.NETWORK_ACTIVE)))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("仮想ネットワークのリスト", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("IPネットワークのリスト ipn", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "ipn", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var ipnets []api.IPNetwork
			err = json.Unmarshal(stdoutStderr, &ipnets)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(ipnets)).To(BeNumerically(">", 0))
		})

		It("仮想ネットワーク配下のIPネットワークのリスト ipn-by-vn", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "network", "ipn-by-vn", netId1, "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "image", "list", "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "image", "create", "--configfile", "testdata/test-image-01-ubuntu22.yaml", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "image", "detail", imageID, "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "image", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
			assertImageListTextHeader(stdoutStderr)
		})
	})

	Context("様々に条件を変えてサーバーのデプロイ", func() {
		It("外部接続の仮想サーバー作成 test-23", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "create", "--configfile", "testdata/test-server-23-test-net-3-host-bridge-ip.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var reply api.Success
			err = json.Unmarshal(stdoutStderr, &reply)
			Expect(err).NotTo(HaveOccurred())
			serverId_1 = reply.Id
		})

		It("外部接続の仮想サーバーの状態確認 test-23", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("外部接続の仮想サーバーの削除 test-23", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("外部接続の仮想サーバーの削除状態確認 test-23", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("外部接続の仮想サーバーで削除を確認 test-23", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			By("仮想サーバーのリスト JSON形式")
			Eventually(func(g Gomega) {
				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "create", "--configfile", configFile, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var reply api.Success
				err = json.Unmarshal(stdoutStderr, &reply)
				Expect(err).NotTo(HaveOccurred())
				switch i {
				case 28:
					serverId_1 = reply.Id
				case 29:
					serverId_2 = reply.Id
				case 30:
					serverId_3 = reply.Id
				}
			}
		})

		It("マルチホームの仮想サーバーの状態確認 test-28", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(db.SERVER_RUNNING))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("マルチホームの仮想サーバーの状態確認 test-29", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_2, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				g.Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err := json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("マルチホームの仮想サーバーの状態確認 test-30", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_3, "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err := json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
				expectServerBootVolumeNodeName(g, server)
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("マルチホームの仮想サーバーの削除 test-28,29,30", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "delete", serverId, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))
			}
		})

		It("マルチホームの仮想サーバーの削除状態確認 test-28,29,30", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				Eventually(func(g Gomega) {
					cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId, "--output", "json")
					stdoutStderr, err := cmd.CombinedOutput()
					fmt.Println(string(stdoutStderr))

					if err != nil {
						// Accept not found as terminal state when deletion finishes before polling.
						if strings.Contains(strings.ToLower(string(stdoutStderr)), "not found") ||
							strings.Contains(strings.ToLower(string(stdoutStderr)), "404") {
							return
						}
						_, dbErr := mockServer.server.Ma.Db.GetServerById(serverId)
						if errors.Is(dbErr, db.ErrNotFound) {
							return
						}
					}
					g.Expect(err).NotTo(HaveOccurred())

					var server api.Server
					err = json.Unmarshal(stdoutStderr, &server)
					Expect(err).NotTo(HaveOccurred())
					g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
				}, 120*time.Second, 5*time.Second).Should(Succeed())
			}
		})

		It("マルチホームの仮想サーバーで削除を確認 test-28,29,30", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			By("仮想サーバーのリスト JSON形式")
			Eventually(func(g Gomega) {
				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "create", "--configfile", configFile, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var reply api.Success
				err = json.Unmarshal(stdoutStderr, &reply)
				Expect(err).NotTo(HaveOccurred())
				switch i {
				case 31:
					serverId_1 = reply.Id
				case 32:
					serverId_2 = reply.Id
				case 33:
					serverId_3 = reply.Id
				}
				time.Sleep(15 * time.Second) // 連続で作成すると、生成順番が入れ替わるため、エラーになることがあるので少し待つ
			}
		})

		It("マルチホームでホストブリッジ接続の仮想サーバーの状態確認 test-31,32,33", func() {
			for i := 31; i <= 33; i++ {
				Eventually(func(g Gomega) {
					serverId := ""
					wantStatus := int(db.SERVER_RUNNING)
					switch i {
					case 31:
						serverId = serverId_1
					case 32:
						serverId = serverId_2
					case 33:
						serverId = serverId_3
						wantStatus = int(db.SERVER_ERROR)
					}
					cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId, "--output", "json")
					stdoutStderr, err := cmd.CombinedOutput()
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(stdoutStderr))

					var server api.Server
					err = json.Unmarshal(stdoutStderr, &server)
					Expect(err).NotTo(HaveOccurred())
					g.Expect(server.Status.StatusCode).To(Equal(wantStatus))
					if wantStatus == int(db.SERVER_RUNNING) {
						expectServerBootVolumeNodeName(g, server)
					} else {
						g.Expect(server.Status).NotTo(BeNil())
						g.Expect(server.Status.Message).NotTo(BeNil())
						g.Expect(*server.Status.Message).To(ContainSubstring("already in use"))
					}
				}, 90*time.Second, 5*time.Second).Should(Succeed())
			}
		})

		It("マルチホームでホストブリッジ接続の仮想サーバーの削除 test-31,32,33", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "delete", serverId, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))
			}
		})

		It("マルチホームでホストブリッジ接続の仮想サーバーの削除状態確認 test-31,32,33", func() {
			for _, serverId := range []string{serverId_1, serverId_2, serverId_3} {
				Eventually(func(g Gomega) {
					cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId, "--output", "json")
					stdoutStderr, err := cmd.CombinedOutput()
					fmt.Println(string(stdoutStderr))
					if err != nil {
						// Accept not found as terminal state when deletion finishes before polling.
						if strings.Contains(strings.ToLower(string(stdoutStderr)), "not found") || strings.Contains(strings.ToLower(string(stdoutStderr)), "404") {
							return
						}
						_, dbErr := mockServer.server.Ma.Db.GetServerById(serverId)
						if errors.Is(dbErr, db.ErrNotFound) {
							return
						}
					}
					Expect(err).NotTo(HaveOccurred())

					var server api.Server
					err = json.Unmarshal(stdoutStderr, &server)
					Expect(err).NotTo(HaveOccurred())
					g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
				}, 120*time.Second, 5*time.Second).Should(Succeed())
			}
		})

		It("マルチホームでホストブリッジ接続の仮想サーバーで削除を確認 test-31,32,33", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			By("仮想サーバーのリスト JSON形式")
			Eventually(func(g Gomega) {
				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "json")
				stdoutStderr, err = cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var servers []api.Server
				err = json.Unmarshal(stdoutStderr, &servers)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(servers)).To(Equal(0))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("SSH-KEYを設定したサーバー作成 test-34", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "create", "--configfile", "testdata/test-server-34-sshkey.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			var reply api.Success
			err = json.Unmarshal(stdoutStderr, &reply)
			Expect(err).NotTo(HaveOccurred())
			serverId_1 = reply.Id
		})

		It("外部接続の仮想サーバーの状態確認 test-34", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(stdoutStderr))

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_RUNNING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("外部接続の仮想サーバーの削除 test-34", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "delete", serverId_1, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))
		})

		It("外部接続の仮想サーバーの削除状態確認 test-34", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "detail", serverId_1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				fmt.Println(string(stdoutStderr))

				if err != nil {
					// Accept not found as terminal state when deletion finishes before polling.
					if strings.Contains(strings.ToLower(string(stdoutStderr)), "not found") {
						return
					}
				}
				Expect(err).NotTo(HaveOccurred())

				var server api.Server
				err = json.Unmarshal(stdoutStderr, &server)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(server.Status.StatusCode).To(Equal(int(db.SERVER_DELETING)))
			}, 120*time.Second, 5*time.Second).Should(Succeed())
		})

		It("外部接続の仮想サーバーで削除を確認 test-34", func() {
			By("仮想サーバーのリスト")
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(stdoutStderr))

			By("仮想サーバーのリスト JSON形式")
			Eventually(func(g Gomega) {
				cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "server", "list", "--output", "json")
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
