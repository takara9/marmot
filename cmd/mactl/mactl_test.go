package main

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
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	BeforeAll(func(ctx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)
		cleanupTestEnvironment()
	})

	AfterAll(func(ctx SpecContext) {
		os.Remove("bin/mactl-test")
		os.Remove("/var/actions-runner/_work/marmot/marmot/cmd/mactl/bin/mactl-test")
		cleanupTestEnvironment()
	})

	Context("基本的なクライアントからのアクセステスト", func() {
		var hvs config.Hypervisors_yaml
		var ctx context.Context
		var cancel context.CancelFunc
		var containerID string
		var marmotServer *marmotd.Server

		It("モックサーバー用etcdの起動", func() {
			cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
			}
			containerID = string(output[:12]) // 最初の12文字をIDとして取得
			fmt.Printf("Container started with ID: %s\n", containerID)
			//time.Sleep(5 * time.Second) // コンテナが起動するまで待機
		})

		It("モックサーバーの起動", func() {
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = startMockServer(ctx)
			//time.Sleep(5 * time.Second) // Marmotインスタンスの生成待ち
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

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
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
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			jsonStr := string(stdoutStderr)
			GinkgoWriter.Print("stdout:", jsonStr)

			var volumes []api.Volume
			if err := json.Unmarshal([]byte(jsonStr), &volumes); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			for _, v := range volumes {
				cmdDel := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "rename", v.Id, "NEW_NAME", "--output", "json")
				stdoutStderr, err := cmdDel.CombinedOutput()
				GinkgoWriter.Print("err=", err)
				GinkgoWriter.Print("stdoutStderr=", string(stdoutStderr))
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("ボリュームのJSONリスト取得 & 削除 26", func() {
			cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "list", "--output", "json")
			stdoutStderr, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			jsonStr := string(stdoutStderr)
			GinkgoWriter.Print("stdout:", jsonStr)

			var volumes []api.Volume
			if err := json.Unmarshal([]byte(jsonStr), &volumes); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			for _, v := range volumes {
				cmdDel := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "volume", "destroy", v.Id, "--output", "json")
				stdoutStderr, err := cmdDel.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Print(string(stdoutStderr))
			}
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

	Context("基本的なCLIからのアクセステスト サーバー編", func() {
		var hvs config.Hypervisors_yaml
		var ctx context.Context
		var cancel context.CancelFunc
		var containerID string
		var marmotServer *marmotd.Server

		It("モックサーバー用etcdの起動", func() {
			cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
			}
			containerID = string(output[:12]) // 最初の12文字をIDとして取得
			fmt.Printf("Container started with ID: %s\n", containerID)
			//time.Sleep(5 * time.Second) // コンテナが起動するまで待機
		})

		It("モックサーバーの起動", func() {
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = startMockServer(ctx)
			//time.Sleep(5 * time.Second) // Marmotインスタンスの生成待ち
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

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
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
			//Expect(err).NotTo(HaveOccurred())
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

		/*
			It("サーバーの更新", func() {
				cmd := exec.Command("./bin/mactl-test", "--api", "testdata/config_marmot.conf", "server", "update", "--output", "json")
				stdoutStderr, err := cmd.CombinedOutput()
				Expect(err).To(HaveOccurred())
				GinkgoWriter.Println(string(stdoutStderr))
			})
		*/

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
})
