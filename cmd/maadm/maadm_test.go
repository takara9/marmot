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
})
