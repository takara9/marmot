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
		marmotPort        = 8110
		etcdPort          = 10379
		etcdctlExe        = "/usr/bin/etcdctl"
		nodeName          = "hvc"
		etcdImage         = "ghcr.io/takara9/etcd:3.6.5"
		etcdContainerName = "etcd-server-image"
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
	)
	etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)
	marmotEp := "localhost:" + fmt.Sprintf("%d", marmotPort)

	BeforeAll(func(ctx0 SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
	})

	AfterAll(func(ctx0 SpecContext) {
		GinkgoWriter.Println("Cleaning up test environment")
		if cancel != nil {
			cancel() // モックサーバーを停止するためにキャンセル関数を呼び出す
		}
		if containerID != "" {
			cmd := exec.Command("docker", "stop", containerID)
			output, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Printf("Failed to stop container: %s, %v\n", string(output), err)
			} else {
				GinkgoWriter.Printf("Container stopped: %s\n", containerID)
			}
		}
		//marmotd.CleanupTestEnvironment()
	})

	Context("テスト環境初期化", func() {
		//var hvs config.Hypervisors_yaml

		It("モックサーバー用etcdの起動", func() {
			cmd := exec.Command("docker", "run", "-d", "--name", etcdContainerName, "-p", fmt.Sprintf("%d", etcdPort)+":2379", "-p", fmt.Sprintf("%d", etcdPort+1)+":2380", "--rm", etcdImage)
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
			}
			containerID = string(output[:12]) // 最初の12文字をIDとして取得
			fmt.Printf("Container started with ID: %s\n", containerID)
		})

		It("モックサーバーの起動", func() {
			GinkgoWriter.Println("Start marmot server mock")
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
		})

		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://"+marmotEp+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		//It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
		//	err := config.ReadYAML("testdata/hypervisor-config-server.yaml", &hvs)
		//	Expect(err).NotTo(HaveOccurred())
		//})

		//It("OSイメージテンプレート", func() {
		//	for _, hd := range hvs.Imgs {
		//		err := marmotServer.Ma.Db.SetImageTemplate(hd)
		//		Expect(err).NotTo(HaveOccurred())
		//	}
		//})

		//It("シーケンス番号のリセット", func() {
		//	for _, sq := range hvs.Seq {
		//		err := marmotServer.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
		//		Expect(err).NotTo(HaveOccurred())
		//	}
		//})

		It("モックサーバー起動の確認", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", etcdUrl+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("LVMの状態確認", func() {
			out, err := exec.Command("lvs", "vg1").Output()
			fmt.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("URLを指定してダウンロードしたイメージからVM起動イメージを作成する", func() {
		var id string
		It("URLを指定してイメージのIDを取得", func() {
			var err error
			GinkgoWriter.Println("URLを指定してイメージのIDを取得")
			url := "https://cloud-images.ubuntu.com/releases/jammy/release-20260218/ubuntu-22.04-server-cloudimg-amd64.img"
			id, err = marmotServer.Ma.Db.MakeImageEntryFromURL("ubuntu-22.04", url)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("取得したイメージID: ", id)
		})

		It("ダウンロードとセットアップ", func() {
			image, err := marmotServer.Ma.CreateNewImageManage(id)
			Expect(err).NotTo(HaveOccurred())
			jsonBytes, err := json.MarshalIndent(image, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("Created image: ", string(jsonBytes))
		})
	})

	Context("URLを指定してダウンロードしたイメージからVM起動イメージを作成する", func() {
		var id string
		It("URLを指定してイメージのIDを取得", func() {
			var err error
			GinkgoWriter.Println("URLを指定してイメージのIDを取得")
			url := "https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img"
			id, err = marmotServer.Ma.Db.MakeImageEntryFromURL("ubuntu-24.04", url)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("取得したイメージID: ", id)
		})

		It("ダウンロードとセットアップ", func() {
			image, err := marmotServer.Ma.CreateNewImageManage(id)
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
