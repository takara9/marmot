package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/types"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var containerID string
	var containerName string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動 - ユニークなコンテナ名を生成
		containerName = fmt.Sprintf("etcd-test-%d", time.Now().UnixNano())
		cmd := exec.Command("docker", "run", "-d", "--name", containerName, "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)

		e := echo.New()
		server := marmotd.NewServer("hvc", "http://127.0.0.1:3379")
		go func() {
			// Setup slog
			opts := &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}
			logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
			slog.SetDefault(logger)

			api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
			fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
		}()
	}, NodeTimeout(20*time.Second))

	AfterAll(func(ctx SpecContext) {
		// Dockerコンテナを停止・削除
		cmd := exec.Command("docker", "stop", containerName)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerName)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}
	}, NodeTimeout(20*time.Second))

	Context("maadm setup の動作テスト", func() {
		var d *db.Database
		var h types.Hypervisor
		It("Marmotd の初期データを、etcdに直接セット", func() {
			cmd := exec.Command("./bin/maadm-test", "setup", "--hvconfig", "testdata/hypervisor-config-hvc.yaml", "--etcdurl", "http://localhost:3379")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("command messeage: ", string(stdoutStderr))
		})

		// marmotd を介さなず、DB操作で内容をチェックする
		It("キーでハイパーバイザーのセットした情報を取得", func() {
			var err error
			d, err = db.NewDatabase("http://localhost:3379")
			Expect(err).NotTo(HaveOccurred())
			h, err = d.GetHypervisorByKey("hvc")
			Expect(err).NotTo(HaveOccurred())
			Expect(h.Nodename).To(Equal("hvc"))
			Expect(h.IpAddr).To(Equal("127.0.0.1"))
			Expect(h.Cpu).To(Equal(4))
			Expect(h.Memory).To(Equal(16384))
			Expect(h.StgPool[0].VolGroup).To(Equal("vg1"))
			Expect(h.StgPool[1].VolGroup).To(Equal("vg2"))
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
		It("mactl version でバージョンを取得", func() {
			cmd := exec.Command("./bin/maadm-test", "version", "--api", "testdata/config_marmot.conf")
			stdoutStderr, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println("client version ", Version)
			GinkgoWriter.Println("server version ", string(stdoutStderr))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("maadm export/import の動作テスト", func() {
		/*
			var d *db.Database
			It("キーでハイパーバイザーのセットした情報を取得", func() {
				var err error
				d, err = db.NewDatabase("http://localhost:3379")
				Expect(err).NotTo(HaveOccurred())
			})
		*/

		It("mactl export 取得", func() {
			cmd := exec.Command("./bin/maadm-test", "export", "--etcdurl", "http://localhost:3379", "--filename", "/tmp/marmot-backup.zip")
			stdout, err := cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println("stdout: ", string(stdout))
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("zipinfo", "-1", "/tmp/marmot-backup.zip")
			stdout, err = cmd.CombinedOutput()
			GinkgoWriter.Println("err: ", err)
			GinkgoWriter.Println("stdout: ", string(stdout))
			line := strings.Split(string(stdout), "\n")
			GinkgoWriter.Println("stdout line= ", len(line))
			Expect(5).To(Equal(len(line)))
		})
	})
})
