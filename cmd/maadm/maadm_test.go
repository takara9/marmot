package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var containerID1 string
	var containerName1 string

	var containerID2 string
	var containerName2 string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動 - ユニークなコンテナ名を生成
		containerName1 = fmt.Sprintf("etcd-test-1-%d", time.Now().UnixNano())
		cmd := exec.Command("docker", "run", "-d", "--name", containerName1, "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID1 = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container1 started with ID: %s\n", containerID1)

		containerName2 = fmt.Sprintf("etcd-test-2-%d", time.Now().UnixNano())
		cmd = exec.Command("docker", "run", "-d", "--name", containerName2, "-p", "4379:2379", "-p", "4380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output2, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID1 = string(output2[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container2 started with ID: %s\n", containerID2)

		e := echo.New()
		server := marmotd.NewServer("hvc", "http://127.0.0.1:3379")
		go func() {
			// Setup slog
			opts := &slog.HandlerOptions{
				AddSource: true,
				//Level:     slog.LevelDebug,
			}
			logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
			slog.SetDefault(logger)

			api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
			fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
		}()
	}, NodeTimeout(20*time.Second))

	AfterAll(func(ctx SpecContext) {
		// Dockerコンテナを停止・削除
		cmd := exec.Command("docker", "stop", containerName1)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerName1)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}

		cmd = exec.Command("docker", "stop", containerName2)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerName2)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}

	}, NodeTimeout(20*time.Second))

	Context("maadm setup の動作テスト", func() {
		var d *db.Database
		var h api.Hypervisor
		It("Marmotd の初期データを、etcdに直接セット", func() {
			cmd := exec.Command("./bin/maadm-test", "setup", "--hvconfig", "testdata/hypervisor-config-hvc.yaml", "--etcdurl", "http://localhost:3379")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("command messeag: ", string(stdoutStderr))
		})

		// marmotd を介さずに、DB操作で内容をチェックする
		It("キーでハイパーバイザーのセットした情報を取得", func() {
			var err error
			d, err = db.NewDatabase("http://localhost:3379")
			Expect(err).NotTo(HaveOccurred())
			h, err = d.GetHypervisorByKey("hvc")
			Expect(err).NotTo(HaveOccurred())
			Expect(h.NodeName).To(Equal("hvc"))
			Expect(*h.IpAddr).To(Equal("127.0.0.1"))
			Expect(h.Cpu).To(Equal(int32(4)))
			Expect(*h.Memory).To(Equal(int64(16384)))
			Expect(*(*h.StgPool)[0].VolGroup).To(Equal("vg1"))
			Expect(*(*h.StgPool)[1].VolGroup).To(Equal("vg2"))
		})

		It("OSイメージ、LVOS、LVDATA、VMのシーケンス番号をチェック", func() {
			// OSイメージのシーケンス番号をチェック
			seq, err := d.GetSeq("LVOS")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(900)))
			seq, err = d.GetSeq("LVOS")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(901)))
			// DATAボリュームのシーケンス番号をチェック
			seq, err = d.GetSeq("LVDATA")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(900)))
			seq, err = d.GetSeq("LVDATA")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(901)))
			// VM番号のシーケンス番号をチェック
			seq, err = d.GetSeq("VM")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(900)))
			seq, err = d.GetSeq("VM")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(901)))
		})

		It("OSイメージのデータ取得チェック", func() {
			vg, lv, err := d.GetOsImgTempByKey("ubuntu22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(vg).To(Equal("vg1"))
			Expect(lv).To(Equal("lv02"))
		})
	})

	Context("maadm version の動作テスト", func() {
		It("maadm version でバージョンを取得", func() {
			cmd := exec.Command("./bin/maadm-test", "version", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("maadm version JSON形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/maadm-test", "version", "--output", "json", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("maadm version TEXT形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/maadm-test", "version", "--output", "text", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

		It("maadm version YAML形式でバージョンを取得", func() {
			cmd := exec.Command("./bin/maadm-test", "version", "--output", "yaml", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println(string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("maadm export/import の動作テスト", func() {
		It("maadm export 取得", func() {
			cmd := exec.Command("./bin/maadm-test", "export", "--etcdurl", "http://localhost:3379", "--filename", "/tmp/marmot-backup.zip")
			stdout, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println("stdout: ", string(stdout))
			Expect(err).NotTo(HaveOccurred())
		})

		It("maadm import インポート", func() {
			cmd := exec.Command("./bin/maadm-test", "import", "--etcdurl", "http://localhost:4379", "--filename", "/tmp/marmot-backup.zip")
			stdout, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println("stdout: ", string(stdout))
			Expect(err).NotTo(HaveOccurred())
		})

		var d *db.Database
		var h api.Hypervisor

		// marmotd を介さずに、DB操作で内容をチェックする
		It("キーでハイパーバイザーのセットした情報を取得", func() {
			var err error
			d, err = db.NewDatabase("http://localhost:4379")
			Expect(err).NotTo(HaveOccurred())
			h, err = d.GetHypervisorByKey("hvc")
			Expect(err).NotTo(HaveOccurred())
			Expect(h.NodeName).To(Equal("hvc"))
			Expect(h.IpAddr).To(Equal("127.0.0.1"))
			Expect(h.Cpu).To(Equal(int32(4)))
			Expect(h.Memory).To(Equal(int64(16384)))
			Expect(*(*h.StgPool)[0].VolGroup).To(Equal("vg1"))
			Expect(*(*h.StgPool)[1].VolGroup).To(Equal("vg2"))
		})

		It("OSイメージ、LVOS、LVDATA、VMのシーケンス番号をチェック", func() {
			By("OSイメージのシーケンス番号をチェック")
			seq, err := d.GetSeq("LVOS")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(902)))
			By("DATAボリュームのシーケンス番号をチェック")
			seq, err = d.GetSeq("LVDATA")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(902)))
			By("VM番号のシーケンス番号をチェック")
			seq, err = d.GetSeq("VM")
			Expect(err).NotTo(HaveOccurred())
			Expect(seq).To(Equal(uint64(902)))
		})

		It("OSイメージのデータ取得チェック", func() {
			vg, lv, err := d.GetOsImgTempByKey("ubuntu22.04")
			Expect(err).NotTo(HaveOccurred())
			Expect(vg).To(Equal("vg1"))
			Expect(lv).To(Equal("lv02"))
		})
	})
})
