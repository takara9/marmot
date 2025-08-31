package main

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmot"
	//clientv3 "go.etcd.io/etcd/client/v3"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var err error
	var containerID string
	//var Conn *clientv3.Client
	var ep *marmot.Marmot

	BeforeAll(func(ctx SpecContext) {
		GinkgoWriter.Println("Start marmot server mock")
		startMockServer() // 戻り値なし
		time.Sleep(5 * time.Second)
		ep, err = marmot.NewMarmot("hvc", "http://127.0.0.1:3379")
		if err != nil {
			GinkgoWriter.Println("Error creating MarmotEndpoint:", err)
		} else {
			GinkgoWriter.Println("MarmotEndpoint created successfully:", ep)
		}

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

	Context("基本的なアクセステスト", func() {
		//const hypervior_config string = "testdata/hypervisor-config-hvc.yaml"
		var hvs config.Hypervisors_yaml

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err = config.ReadYAML("testdata/hypervisor-config-hvc.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
