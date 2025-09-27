package marmotd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmot"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var err error
	var containerID string
	var marmotServer *Server

	BeforeAll(func(ctx SpecContext) {
		// Marmotサーバーのモック起動
		GinkgoWriter.Println("Start marmot server mock")
		marmotServer = startMockServer() // バックグラウンドで起動する
		time.Sleep(5 * time.Second)      // Marmotインスタンスの生成待ち

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "-e", "ALLOW_NONE_AUTHENTICATION=yes", "-e", "ETCD_ADVERTISE_CLIENT_URLS=http://127.0.0.1:3379", "bitnami/etcd")
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
	}, NodeTimeout(20*time.Second))

	Context("基本的なクライアントからのアクセステスト", func() {
		var hvs config.Hypervisors_yaml
		var marmotClient *marmot.MarmotEndpoint

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err = config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Marmotエンドポイントの生成", func() {
			marmotClient, err = marmot.NewMarmotdEp(
				"http",
				"localhost:8080",
				"/api/v1",
				60,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				fmt.Println(hv)
				err := marmotServer.Ma.Db.SetHypervisor(hv)
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

		It("Marmotd の生存確認", func() {
			httpStatus, body, url, err := marmotClient.Ping()
			var replyMessage api.ReplyMessage
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(200))
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(replyMessage.Message).To(Equal("ok"))
			Expect(url).To(BeNil())
		})

		It("Marmotd のバージョン情報取得", func() {
			body, err := marmotClient.GetVersion()
			//var version api.Version
			Expect(err).NotTo(HaveOccurred())
			//Expect(httpStatus).To(Equal(200))
			//err = json.Unmarshal(body, &version)
			Expect(err).NotTo(HaveOccurred())
			Expect(body.Version).To(Equal("0.0.1"))
			//Expect(url).To(BeNil())
		})

		It("ハイパーバイザーの一覧取得", func() {
			httpStatus, body, url, err := marmotClient.ListHypervisors(nil)
			var hvs []api.Hypervisor
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(200))
			err = json.Unmarshal(body, &hvs)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(hvs)).To(Equal(1))
			Expect(hvs[0].NodeName).To(Equal("hvc"))
			Expect(*hvs[0].FreeCpu).To(Equal(int32(4)))
			Expect(url).To(BeNil())
		})

		It("ハイパーバイザーの情報取得", func() {
			httpStatus, body, url, err := marmotClient.GetHypervisor("hvc")
			var hv api.Hypervisor
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(http.StatusOK))
			err = json.Unmarshal(body, &hv)
			Expect(err).NotTo(HaveOccurred())
			Expect(hv.NodeName).To(Equal("hvc"))
			Expect(*hv.FreeCpu).To(Equal(int32(4)))
			Expect(url).To(BeNil())
		})

		It("存在しないハイパーバイザーの情報取得", func() {
			httpStatus, body, url, err := marmotClient.GetHypervisor("hvc-noexist")
			var replyMessage api.ReplyMessage
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(http.StatusNotFound))
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(replyMessage.Message).To(Equal("Hypervisor hvc-noexist not found"))
			Expect(url).To(BeNil())
		})

		var cnf config.MarmotConfig
		It("Load Config", func() {
			fn := "testdata/cluster-config.yaml"
			ccf := &fn
			err := config.ReadConfig(*ccf, &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("クラスタの生成", func() {
			httpStatus, body, url, err := marmotClient.CreateCluster(cnf)
			GinkgoWriter.Println("CreateCluster ERR = ", err)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(201))
			var replyMessage api.ReplyMessage
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())
		})

		It("仮想マシンの一覧取得", func() {
			httpStatus, body, url, err := marmotClient.ListVirtualMachines(nil)
			var vms []api.VirtualMachine
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(200))
			err = json.Unmarshal(body, &vms)
			GinkgoWriter.Println("err = ", err)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vms)).To(Equal(2))
			Expect(url).To(BeNil())
		})

		It("クラスタの一時停止", func() {
			httpStatus, body, url, err := marmotClient.StopCluster(cnf)
			GinkgoWriter.Println("StopCluster ERR = ", err)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(201))
			var replyMessage api.ReplyMessage
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())
			time.Sleep(time.Second * 120)
		})

		It("クラスタの再スタート", func() {
			httpStatus, body, url, err := marmotClient.StartCluster(cnf)
			GinkgoWriter.Println("StartCluster ERR = ", err)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(201))
			var replyMessage api.ReplyMessage
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())
		})

		It("クラスタの削除", func() {
			httpStatus, body, url, err := marmotClient.DestroyCluster(cnf)
			GinkgoWriter.Println("DestroyCluster ERR = ", err)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(200))
			var replyMessage api.ReplyMessage
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())
		})
	})
})
