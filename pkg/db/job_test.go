package db_test

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Jobs", Ordered, func() {
	var url string
	var err error
	var d *db.Database
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Setup slog
		opts := &slog.HandlerOptions{
			AddSource: true,
			//Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		// Dockerコンテナを起動
		url = "http://127.0.0.1:5379"
		cmd := exec.Command("docker", "run", "-d", "--name", "jobEtcdDb", "-p", "5379:2379", "-p", "5380:2380", "ghcr.io/takara9/etcd:3.6.5")
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
		fmt.Println("STOPPING CONTAINER:", containerID)
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

	BeforeEach(func() {
	})

	AfterEach(func() {
	})

	Describe("Test etcd", func() {
		Context("Test Connection to etcd", func() {
			It("Connection etcd", func() {
				d, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Job Control", func() {
			It("Entry new job", func() {
				jobTask := "sleep 3"
				jobId, err := d.RegisterJob(jobTask)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
			})
		})
	})
})
