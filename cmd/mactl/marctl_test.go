package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var err error
	var containerID string
	var marmotServer *marmotd.Server

	BeforeAll(func(ctx SpecContext) {
		// Marmotサーバーのモック起動
		GinkgoWriter.Println("Start marmot server mock")
		marmotServer = startMockServer() // バックグラウンドで起動する
		time.Sleep(5 * time.Second)      // Marmotインスタンスの生成待ち

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
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
		os.Remove("bin/mactl-test")
		os.Remove("/var/actions-runner/_work/marmot/marmot/cmd/mactl/bin/mactl-test")
	}, NodeTimeout(20*time.Second))

	Context("基本的なクライアントからのアクセステスト", func() {
		var hvs config.Hypervisors_yaml
		//var marmotClient *marmot.MarmotEndpoint

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err = config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				fmt.Println(hv)
				err := marmotServer.Ma.Db.SetHypervisors(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := marmotServer.Ma.Db.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のリセット", func() {
			for _, sq := range hvs.Seq {
				err := marmotServer.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("ストレージの空き容量チェック", func() {
			err = util.CheckHvVgAll(marmotServer.Ma.EtcdUrl, marmotServer.Ma.NodeName)
			Expect(err).NotTo(HaveOccurred())
		})

		// mactl2 コマンドに置き換え　実装が必要
		It("Marmotd の生存確認", func() {
			/*
				httpStatus, body, url, err := marmotClient.Ping()
				var replyMessage api.ReplyMessage
				Expect(err).NotTo(HaveOccurred())
				Expect(httpStatus).To(Equal(200))
				err = json.Unmarshal(body, &replyMessage)
				Expect(err).NotTo(HaveOccurred())
				Expect(replyMessage.Message).To(Equal("ok"))
				Expect(url).To(BeNil())
			*/
		})

		It("Marmotd のバージョン情報取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "version")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
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

		It("ハイパーバイザーの一覧取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "hv")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("グローバルステータス取得", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "globalStatus")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("クラスタ 1 の生成", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "create", "-c", "testdata/cluster-config1.yaml")
			//stdoutStderr, err := cmd.CombinedOutput()
			_, err := cmd.CombinedOutput()
			//GinkgoWriter.Println("stdout =", string(stdoutStderr))
			//GinkgoWriter.Println("err = ", string(err.Error()))
			Expect(err).NotTo(HaveOccurred())
		})

		It("仮想マシンの一覧取得-1", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "status", "-c", "testdata/cluster-config1.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("クラスタ1の削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "destroy", "-c", "testdata/cluster-config1.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("クラスタ2の生成", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "create", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("仮想マシンの一覧取得-2", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "status", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("クラスタ2の一時停止", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "stop", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("仮想マシンの一覧取得-3", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "status", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("クラスタ2の再スタート", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "start", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("仮想マシンの一覧取得-4", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "status", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("クラスタ2の削除", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "destroy", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("仮想マシンの一覧取得-5", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "status", "-c", "testdata/cluster-config2.yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})
	})
})
