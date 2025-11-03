package main

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
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

	Context("ハイパーバイザーのシステム管理操作", func() {
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
		})
	})
})
