package main

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	//. "github.com/onsi/gomega"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var containerID string
	var containerName string
	//var marmotServer *marmotd.Server

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
		time.Sleep(10 * time.Second) // コンテナが起動するまで待機
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

	Context("基本的なクライアントからのアクセステスト", func() {
		It("コマンドの構成ファイルの読み取り", func() {
			GinkgoWriter.Println("Reading config file")
		})

	})
})
