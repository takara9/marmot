package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var err error
	var containerID string
	var marmotServer *marmotd.Server

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)
		time.Sleep(5 * time.Second) // コンテナが起動するまで待機

		// Marmotサーバーのモック起動
		GinkgoWriter.Println("Start marmot server mock")
		marmotServer = startMockServer() // バックグラウンドで起動する
		time.Sleep(5 * time.Second)      // Marmotインスタンスの生成待ち
	}, NodeTimeout(15*time.Second))

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

		cmd = exec.Command("lvremove vg1/oslv0900 -y")
		cmd.CombinedOutput()
		cmd = exec.Command("lvremove vg1/oslv0901 -y")
		cmd.CombinedOutput()
		cmd = exec.Command("lvremove vg1/oslv0902 -y")
		cmd.CombinedOutput()

		cmd = exec.Command("lvremove vg2/data0900 -y")
		cmd.CombinedOutput()
		cmd = exec.Command("lvremove vg2/data0901 -y")
		cmd.CombinedOutput()
		cmd = exec.Command("lvremove vg2/data0902 -y")
		cmd.CombinedOutput()
		cmd = exec.Command("lvremove vg2/data0903 -y")
		cmd.CombinedOutput()

	}, NodeTimeout(30*time.Second))

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
			cmd0 := exec.Command("pwd")
			stdoutStderr0, err := cmd0.CombinedOutput()
			GinkgoWriter.Println(string(stdoutStderr0))

			cmd1 := exec.Command("ls", "-lR")
			stdoutStderr1, err := cmd1.CombinedOutput()
			GinkgoWriter.Println(string(stdoutStderr1))

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
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
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

		It("ボリュームのリスト取得  00", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの作成  data qcow2 26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume1", "-t", "qcow2", "-k", "data", "-s", "2", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの作成  data lvm 26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume2", "-t", "lvm", "-k", "data", "-s", "1", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの作成  os qcow2 26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume3", "-t", "qcow2", "-k", "os", "-l", "ubuntu22.04", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームの作成  os lvm 26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "create", "-n", "test-volume4", "-t", "lvm", "-k", "os", "-l", "ubuntu22.04", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのTEXTリスト取得  26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "text")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのJSONリスト取得  26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリュームのYAMLリスト取得  26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "yaml")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println(string(stdoutStderr))
		})

		It("ボリューム名変更", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
			Expect(err).NotTo(HaveOccurred())
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			jsonStr := string(stdoutStderr)
			GinkgoWriter.Print("stdout:", jsonStr)

			var volumes []api.Volume
			if err := json.Unmarshal([]byte(jsonStr), &volumes); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			for _, v := range volumes {
				cmdDel := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "rename", *v.Key, "NEW_NAME", "--output", "json")
				stdoutStderr, err := cmdDel.CombinedOutput()
				exitErr, ok := err.(*exec.ExitError)
				GinkgoWriter.Println("Exit Code:", exitErr.ExitCode())
				GinkgoWriter.Println("Exit Code ok?", ok)
				Expect(exitErr.ExitCode()).To(Equal(0))
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Print(string(stdoutStderr))
			}
		})

		It("ボリュームのJSONリスト取得＆削除 26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
			Expect(err).NotTo(HaveOccurred())
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			jsonStr := string(stdoutStderr)
			GinkgoWriter.Print("stdout:", jsonStr)

			var volumes []api.Volume
			if err := json.Unmarshal([]byte(jsonStr), &volumes); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			for _, v := range volumes {
				cmdDel := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "destroy", *v.Key, "--output", "json")
				stdoutStderr, err := cmdDel.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Print(string(stdoutStderr))
			}
		})
	})
})
