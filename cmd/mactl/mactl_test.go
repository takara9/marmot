package main

import (
	"encoding/json"
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

const testServerConfigRawURL = "https://raw.githubusercontent.com/takara9/marmot/refs/heads/config-server-to-api-server/cmd/mactl/testdata/test-server-1.yaml"

var _ = Describe("Marmotd Test", Ordered, func() {
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
		cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
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
	})

	Context("クライアントからアクセステスト", func() {
		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8080/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("Marmotd のバージョン情報取得", func() {
			cmd0 := exec.Command("pwd")
			stdoutStderr0, err := cmd0.CombinedOutput()
			GinkgoWriter.Println(string(stdoutStderr0))

			cmd1 := exec.Command("ls", "-lR")
			stdoutStderr1, err := cmd1.CombinedOutput()
			GinkgoWriter.Println(string(stdoutStderr1))

			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "version")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(stdoutStderr))

			Expect(err).NotTo(HaveOccurred())
		})

		It("mactl version JSON形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/mactl-test", "version", "--output", "json", "--api", "testdata/.marmot")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("mactl version TEXT形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/mactl-test", "version", "--output", "text", "--api", "testdata/.marmot")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("mactl version YAML形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/mactl-test", "version", "--output", "yaml", "--api", "testdata/.marmot")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("クライアントからのアクセステスト ボリューム編", func() {
		It("ボリュームのリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		var volumeID string
		It("ボリュームの作成  data qcow2 2G", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-01-data-qcow2.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("test-volume1"))
			Expect(*volume.Spec.Size).To(Equal(int(2)))
			volumeID = volume.Id
			fmt.Println("Volume ID:", volumeID)
		})

		It("ボリュームの個別詳細取得", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_AVAILABLE))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームの削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "delete", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			By("削除したボリュームの状態を確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_DELETING))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームの作成  data lvm 2G", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-02-data-lvm.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("test-volume2"))
			Expect(*volume.Spec.Size).To(Equal(int(2)))
			volumeID = volume.Id
			fmt.Println("Volume ID:", volumeID)
		})

		It("ボリュームの個別詳細取得", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_AVAILABLE))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームの削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "delete", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			By("削除したボリュームの状態を確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_DELETING))
			}, 120*time.Second, 3*time.Second).Should(Succeed())

			By("削除したボリュームが消えることを確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volumes []api.Volume
				err = json.Unmarshal(stdoutStderr, &volumes)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(volumes)).To(Equal(0))
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})

		var volumeID3 string
		It("ボリュームの作成  os qcow2 失敗ケース", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-03-os-qcow2.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("test-volume3"))
			Expect(*volume.Spec.Size).To(Equal(int(16)))
			volumeID3 = volume.Id
			fmt.Println("Volume ID:", volumeID3)
		})

		var volumeID4 string
		It("ボリュームの作成  os lvm 失敗ケース", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-04-os-lvm.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("test-volume4"))
			Expect(*volume.Spec.Size).To(Equal(int(16)))
			volumeID4 = volume.Id
			fmt.Println("Volume ID:", volumeID4)
		})

		It("ボリュームのリスト取得 ", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()

				Expect(err).NotTo(HaveOccurred())
				fmt.Println("stdout:", string(stdoutStderr))
				var volumes []api.Volume
				if err := json.Unmarshal(stdoutStderr, &volumes); err != nil {
					Expect(err).NotTo(HaveOccurred())
				}
				GinkgoWriter.Println("Retrieved volumes:")
				for _, v := range volumes {
					GinkgoWriter.Printf("  - %s (%s)\n", *v.Metadata.Name, v.Id)
				}
				// ボリュームがエラー状態になるまで待つ
				g.Expect(volumes).To(HaveLen(2))
				g.Expect(volumes[0].Status.StatusCode).To(Equal(db.VOLUME_ERROR))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームのTEXTリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "delete", volumeID3, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "delete", volumeID4, "--output", "json")
			stdoutStderr, err = cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのリスト取得 削除され無くなることを確認", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()

				Expect(err).NotTo(HaveOccurred())
				fmt.Println("stdout:", string(stdoutStderr))
				var volumes []api.Volume
				if err := json.Unmarshal(stdoutStderr, &volumes); err != nil {
					Expect(err).NotTo(HaveOccurred())
				}
				GinkgoWriter.Println("Retrieved volumes:")
				for _, v := range volumes {
					GinkgoWriter.Printf("  - %s (%s)\n", *v.Metadata.Name, v.Id)
				}
				// ボリュームがエラー状態になるまで待つ
				g.Expect(volumes).To(HaveLen(0))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームのリスト取得 テキスト形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのリスト取得 JSON形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのリスト取得 YAML形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの作成 名前変更用", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-05-rename-source.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("test-origine"))
			Expect(*volume.Spec.Size).To(Equal(int(2)))
			volumeID = volume.Id
			fmt.Println("Volume ID:", volumeID)
		})

		It("ボリュームの個別詳細取得", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_AVAILABLE))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリューム名変更", func() {
			cmdDel := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "rename", volumeID, "NEW-NAME", "--output", "json")
			stdoutStderr, err := cmdDel.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Print("stdoutStderr=", string(stdoutStderr))
			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())
			Expect(*volume.Metadata.Name).To(Equal("NEW-NAME"))

			By("ボリューム名変更後も metadata.nodeName が保持されることを確認")
			cmdDetail := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
			stdoutStderr, err = cmdDetail.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			var detail api.Volume
			err = json.Unmarshal(stdoutStderr, &detail)
			Expect(err).NotTo(HaveOccurred())
			Expect(detail.Metadata).NotTo(BeNil())
			Expect(detail.Metadata.NodeName).NotTo(BeNil())
			Expect(strings.TrimSpace(*detail.Metadata.NodeName)).NotTo(BeEmpty())
		})

		It("ボリュームの削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "delete", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			By("削除したボリュームの状態を確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_DELETING))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			By("削除したボリュームがリストから無くなることを確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volumes []api.Volume
				err = json.Unmarshal(stdoutStderr, &volumes)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(volumes)).To(Equal(0))

				// デバッグ用に現在のボリュームの状態を出力
				x, err := json.MarshalIndent(volumes, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Current volumes:", string(x))

				// ボリュームがエラー状態になるまで待つ

			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームのリスト取得 テキスト形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
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

		var volumeID3 string
		It("ボリュームの作成  os qcow2 成功ケース", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-06-boot-qcow2.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("boot-volume1"))
			Expect(*volume.Spec.Size).To(Equal(int(16)))
			volumeID3 = volume.Id
			fmt.Println("Volume ID:", volumeID3)
		})

		var volumeID4 string
		It("ボリュームの作成  os lvm 成功ケース", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "create", "-f", "testdata/test-volume-07-os-lvm-success.yaml", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())

			Expect(*volume.Metadata.Name).To(Equal("test-volume2"))
			Expect(*volume.Spec.Size).To(Equal(int(16)))
			volumeID4 = volume.Id
			fmt.Println("Volume ID:", volumeID4)
		})

		It("OSイメージのリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "image", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
			assertImageListTextHeader(stdoutStderr)
		})

		It("ボリュームリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})
	})

	Context("ep コマンドのテスト（エンドポイント管理）", func() {
		var tmpHome string

		// 各テストごとに独立した一時 HOME ディレクトリを用意する
		BeforeEach(func() {
			var err error
			tmpHome, err = os.MkdirTemp("", "mactl-ep-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpHome)
		})

		// HOME を上書きした環境変数リストを返すヘルパー
		epEnv := func() []string {
			env := make([]string, 0, len(os.Environ()))
			for _, e := range os.Environ() {
				if !strings.HasPrefix(e, "HOME=") {
					env = append(env, e)
				}
			}
			return append(env, "HOME="+tmpHome)
		}

		It("エンドポイント未登録のとき ep list はメッセージを表示して正常終了する", func() {
			cmd := exec.Command("./bin/mactl-test", "ep", "list")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("登録されていません"))
		})

		It("ep add でエンドポイントを追加できる", func() {
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "http://localhost:8750")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("http://localhost:8750"))
		})

		It("ep add で複数エンドポイントを追加すると ep list に全件表示される", func() {
			for _, u := range []string{
				"http://localhost:8750",
				"http://hv1:8750",
				"http://hv2:8750",
			} {
				cmd := exec.Command("./bin/mactl-test", "ep", "add", u)
				cmd.Env = epEnv()
				_, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
			}

			cmd := exec.Command("./bin/mactl-test", "ep", "list")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("http://localhost:8750"))
			Expect(string(output)).To(ContainSubstring("http://hv1:8750"))
			Expect(string(output)).To(ContainSubstring("http://hv2:8750"))
			// 最初に追加したエンドポイントがアクティブ
			Expect(string(output)).To(MatchRegexp(`\* active\s+http://localhost:8750`))
		})

		It("ep add で同一 URL を重複登録するとエラーになる", func() {
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "http://localhost:8750")
			cmd.Env = epEnv()
			_, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("./bin/mactl-test", "ep", "add", "http://localhost:8750")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("既に登録済み"))
		})

		It("ep add で不正な URL はエラーになる", func() {
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "not-a-valid-url")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("無効なURL"))
		})

		It("ep set でアクティブなエンドポイントを切り替えられる", func() {
			for _, u := range []string{
				"http://localhost:8750",
				"http://hv1:8750",
			} {
				cmd := exec.Command("./bin/mactl-test", "ep", "add", u)
				cmd.Env = epEnv()
				_, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
			}

			// 2番に切り替え
			cmd := exec.Command("./bin/mactl-test", "ep", "set", "2")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("http://hv1:8750"))

			// リストで hv1 が active になっていることを確認
			cmd = exec.Command("./bin/mactl-test", "ep", "list")
			cmd.Env = epEnv()
			output, err = cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(MatchRegexp(`\* active\s+http://hv1:8750`))
		})

		It("ep set で範囲外の番号はエラーになる", func() {
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "http://localhost:8750")
			cmd.Env = epEnv()
			_, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("./bin/mactl-test", "ep", "set", "99")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("範囲外"))
		})

		It("ep set で数値以外の引数はエラーになる", func() {
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "http://localhost:8750")
			cmd.Env = epEnv()
			_, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("./bin/mactl-test", "ep", "set", "abc")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).To(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("不正"))
		})

		It("ep delete でエンドポイントを削除できる", func() {
			for _, u := range []string{
				"http://localhost:8750",
				"http://hv1:8750",
				"http://hv2:8750",
			} {
				cmd := exec.Command("./bin/mactl-test", "ep", "add", u)
				cmd.Env = epEnv()
				_, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
			}

			// 2番 (hv1:8750) を削除
			cmd := exec.Command("./bin/mactl-test", "ep", "delete", "2")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("http://hv1:8750"))

			// リストに hv1 が含まれていないことを確認
			cmd = exec.Command("./bin/mactl-test", "ep", "list")
			cmd.Env = epEnv()
			output, err = cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).NotTo(ContainSubstring("http://hv1:8750"))
			Expect(string(output)).To(ContainSubstring("http://localhost:8750"))
			Expect(string(output)).To(ContainSubstring("http://hv2:8750"))
		})

		It(".marmot のアクティブエンドポイントを使ってサーバーに接続できる", func() {
			// モックサーバー (http://localhost:8080) をエンドポイントとして登録してアクティブにする
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "http://localhost:8080")
			cmd.Env = epEnv()
			_, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			// --api フラグなしで version コマンドを実行 → .marmot のアクティブ EP が使われる
			cmd = exec.Command("./bin/mactl-test", "version")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
		})

		It("--api フラグは .marmot より優先される", func() {
			// .marmot に到達不能なエンドポイントを登録しておく
			cmd := exec.Command("./bin/mactl-test", "ep", "add", "http://unreachable-host:9999")
			cmd.Env = epEnv()
			_, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			// --api でモックサーバーを明示指定すれば成功する
			cmd = exec.Command("./bin/mactl-test", "--api", "testdata/.marmot", "version")
			cmd.Env = epEnv()
			output, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(output))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
