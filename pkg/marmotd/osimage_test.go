package marmotd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("ImageManagmentTest", Ordered, func() {
	const (
		marmotPort  = 8110
		etcdPort    = 10379
		etcdctlExe  = "/usr/bin/etcdctl"
		nodeName    = "hvc"
		etcdImage   = "ghcr.io/takara9/etcd:3.6.5"
		osImage1    = "ubuntu-22.04-server-cloudimg-amd64.img"
		osImage1URL = "http://hmc/" + osImage1
		osImage2    = "ubuntu-24.04-server-cloudimg-amd64.img"
		osImage2URL = "http://hmc/" + osImage2
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
		osImageid1   string
		osImageid2   string
	)
	marmotEp := "localhost:" + fmt.Sprintf("%d", marmotPort)

	BeforeAll(func(ctx0 SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		By("開始 etcd container")
		cmd := exec.Command("docker", "run", "-d", "-p", fmt.Sprintf("%d", etcdPort)+":2379", "--rm", etcdImage)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)

		By("モックサーバーの起動")
		ctx, cancel = context.WithCancel(context.Background())
		marmotServer = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する

		By("モックサーバーの起動チェック")
		Eventually(func(g Gomega) {
			cmd := exec.Command("curl", "http://"+marmotEp+"/ping")
			err := cmd.Run()
			GinkgoWriter.Println(cmd, "err= ", err)
			g.Expect(err).NotTo(HaveOccurred())
		}).Should(Succeed())

	})

	AfterAll(func(ctx0 SpecContext) {
		GinkgoWriter.Println("Cleaning up test environment")
		By("mockサーバーの停止")
		cancel() // モックサーバーを停止するためにキャンセル関数を呼び出す

		By("etcdコンテナの停止")
		cmd := exec.Command("docker", "kill", containerID)
		err := cmd.Run()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("テスト環境初期化", func() {
		It("LVMの状態確認", func() {
			out, err := exec.Command("lvs", "vg1").Output()
			fmt.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("URLを指定してダウンロードしたイメージからVM起動イメージを作成する", func() {
		It("URLを指定してイメージのIDを取得", func() {
			var err error
			GinkgoWriter.Println("URLを指定してイメージのIDを取得")
			osImageid1, err = marmotServer.Ma.Db.MakeImageEntryFromURL("ubuntu-22.04", osImage1URL)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("取得したイメージID: ", osImageid1)
		})

		It("ダウンロードとセットアップ", func() {
			image, err := marmotServer.Ma.CreateNewImageManage(osImageid1)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image: ", string(jsonBytes))
		})
	})

	Context("URLを指定してダウンロードしたイメージからVM起動イメージを作成する", func() {
		It("URLを指定してイメージのIDを取得", func() {
			var err error
			GinkgoWriter.Println("URLを指定してイメージのIDを取得")
			osImageid2, err = marmotServer.Ma.Db.MakeImageEntryFromURL("ubuntu-24.04", osImage2URL)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("取得したイメージID: ", osImageid2)
		})

		It("ダウンロードとセットアップ", func() {
			image, err := marmotServer.Ma.CreateNewImageManage(osImageid2)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image: ", string(jsonBytes))
		})
	})

	Context("イメージのリスト取得と個別詳細取得、削除", func() {
		var images []api.Image

		It("イメージのリスト取得", func() {
			var err error
			images, err = marmotServer.Ma.GetImagesManage()
			Expect(err).NotTo(HaveOccurred())
			for _, image := range images {
				fmt.Println("ID", image.Id, "Name", *image.Metadata.Name)
			}
		})

		It("イメージの詳細情報取得 -1", func() {
			image, err := marmotServer.Ma.GetImageManage((images)[0].Id)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "    ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(jsonBytes))
		})

		It("イメージの詳細情報取得 -2", func() {
			image, err := marmotServer.Ma.GetImageManage((images)[1].Id)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "    ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(jsonBytes))
		})

		It("イメージの削除 -1", func() {
			err := marmotServer.Ma.DeleteImageManage((images)[0].Id)
			Expect(err).NotTo(HaveOccurred())
			_, err = marmotServer.Ma.GetImageManage((images)[0].Id)
			Expect(err).To(HaveOccurred())
			fmt.Println("Deleted image ID: ", (images)[0].Id)
		})

		It("イメージの削除 -2", func() {
			err := marmotServer.Ma.DeleteImageManage((images)[1].Id)
			Expect(err).NotTo(HaveOccurred())
			_, err = marmotServer.Ma.GetImageManage((images)[1].Id)
			Expect(err).To(HaveOccurred())
			fmt.Println("Deleted image ID: ", (images)[1].Id)
		})
	})
})
