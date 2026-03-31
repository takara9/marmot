package main

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
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var ctx context.Context
	var cancel context.CancelFunc
	var containerID string

	BeforeAll(func(specCtx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
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
		ctx, cancel = context.WithCancel(context.Background())
		startMockServer(ctx)
	})

	AfterAll(func(specCtx SpecContext) {
		cancel() // モックサーバー停止
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

			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "version")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println(string(stdoutStderr))

			Expect(err).NotTo(HaveOccurred())
		})

		It("mactl version JSON形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/mactl-test", "version", "--output", "json", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("mactl version TEXT形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/mactl-test", "version", "--output", "text", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("mactl version YAML形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/mactl-test", "version", "--output", "yaml", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("クライアントからのアクセステスト ボリューム編", func() {
		It("ボリュームのリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		var volumeID string
		It("ボリュームの作成  data qcow2 2G", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume1", "-t", "qcow2", "-k", "data", "-s", "2", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "detail", volumeID, "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "delete", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			By("削除したボリュームの状態を確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_DELETING))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームの作成  data lvm 2G", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume2", "-t", "lvm", "-k", "data", "-s", "2", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "detail", volumeID, "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "delete", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			By("削除したボリュームの状態を確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_DELETING))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			By("削除したボリュームが消えることを確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volumes []api.Volume
				err = json.Unmarshal(stdoutStderr, &volumes)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(volumes)).To(Equal(0))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		var volumeID3 string
		It("ボリュームの作成  os qcow2 失敗ケース", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume3", "-t", "qcow2", "-k", "os", "-l", "ubuntu22.04", "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume4", "-t", "lvm", "-k", "os", "-l", "ubuntu22.04", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "delete", volumeID3, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			cmd = exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "delete", volumeID4, "--output", "json")
			stdoutStderr, err = cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのリスト取得 削除され無くなることを確認", func() {
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのリスト取得 JSON形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのリスト取得 YAML形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの作成 名前変更用", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-origine", "-t", "lvm", "-k", "data", "-s", "2", "--output", "json")
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
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "detail", volumeID, "--output", "json")
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
			cmdDel := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "rename", volumeID, "NEW-NAME", "--output", "json")
			stdoutStderr, err := cmdDel.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Print("stdoutStderr=", string(stdoutStderr))
			var volume api.Volume
			err = json.Unmarshal(stdoutStderr, &volume)
			Expect(err).NotTo(HaveOccurred())
			Expect(*volume.Metadata.Name).To(Equal("NEW-NAME"))
		})

		It("ボリュームの削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "delete", volumeID, "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))

			By("削除したボリュームの状態を確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "detail", volumeID, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volume api.Volume
				err = json.Unmarshal(stdoutStderr, &volume)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(volume.Status.StatusCode).To(Equal(db.VOLUME_DELETING))
			}, 60*time.Second, 3*time.Second).Should(Succeed())

			By("削除したボリュームがリストから無くなることを確認")
			Eventually(func(g Gomega) {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				var volumes []api.Volume
				err = json.Unmarshal(stdoutStderr, &volumes)
				Expect(err).NotTo(HaveOccurred())
				g.Expect(len(volumes)).To(Equal(0))
			}, 60*time.Second, 3*time.Second).Should(Succeed())
		})

		It("ボリュームのリスト取得 テキスト形式", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
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
			url := "https://cloud-images.ubuntu.com/releases/jammy/release-20260218/ubuntu-22.04-server-cloudimg-amd64.img"
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

		var volumeID3 string
		It("ボリュームの作成  os qcow2 成功ケース", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "boot-volume1", "-t", "qcow2", "-k", "os", "-l", "ubuntu22.04", "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume2", "-t", "lvm", "-k", "os", "-l", "ubuntu22.04", "--output", "json")
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
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "image", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームリスト取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

	})

	/*
		Context("基本的なCLIからのアクセステスト サーバー編", func() {
			var ctx context.Context
			var cancel context.CancelFunc
			var containerID string

			It("モックサーバー用etcdの起動", func() {
				cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
				output, err := cmd.CombinedOutput()
				if err != nil {
					Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
				}
				containerID = string(output[:12]) // 最初の12文字をIDとして取得
				fmt.Printf("Container started with ID: %s\n", containerID)
			})

			It("モックサーバーの起動", func() {
				ctx, cancel = context.WithCancel(context.Background())
				startMockServer(ctx)
			})

			It("モックサーバー起動の確認", func() {
				By("Trying to connect to marmot")
				Eventually(func(g Gomega) {
					cmd := exec.Command("curl", "http://localhost:8080/ping")
					err := cmd.Run()
					GinkgoWriter.Println(cmd, "err= ", err)
					g.Expect(err).NotTo(HaveOccurred())
				}).Should(Succeed())
			})

			var id1 string
			It("サーバー単体の作成", func() {
				// このコマンドで、marmotd側でエラーが発生する。
				// エラーが発生する理由は、サーバー生成部分が未実装のため
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--output", "json", "--configfile", "testdata/test-server-1.yaml")
				stdoutStderr, _ := cmd.CombinedOutput()
				GinkgoWriter.Println(string(stdoutStderr))
				var resp api.Success
				err := json.Unmarshal(stdoutStderr, &resp)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("server id:", resp.Id)
				id1 = resp.Id
				Expect(len(resp.Id)).To(BeNumerically(">", 0))
			})

			var id2 string
			It("サーバークラスタの作成", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "create", "--output", "json", "--configfile", "testdata/test-server-2.yaml")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
				var resp api.Success
				err = json.Unmarshal(stdoutStderr, &resp)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println("server id:", resp.Id)
				id2 = resp.Id
				fmt.Println("id2=", id2)
				Expect(len(resp.Id)).To(BeNumerically(">", 0))
			})

			It("サーバーのリスト取得 text", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "text")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーのリスト取得 json", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーのリスト取得 yaml", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "yaml")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーの削除 id1", func() {
				fmt.Println("Deleting server id1=", id1)
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "delete", id1, "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーの個別詳細取得 yaml", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", id2, "--output", "yaml")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーの名前変更", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "update", id2, "--name", "new-server-name", "--output", "yaml")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーの個別詳細取得 yaml", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "detail", id2, "--output", "yaml")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("サーバーのリスト取得", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "list", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})

			It("モックの停止", func() {
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
	*/
})
