package main

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
)

var _ = Describe("Marmotd Test", Ordered, func() {
	var err error
	var containerID string
	//var marmotServer *marmotd.Server

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
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
		var cnf config.DefaultConfig
		var d *db.Database
		//var marmotClient *marmot.MarmotEndpoint
		var hvsOld []types.HypervisorOld
		var hvsNew []types.Hypervisor

		It("コマンドの構成ファイルの読み取り", func() {
			err = config.ReadYAML("testdata/config_marmot.conf", &cnf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadConfig("testdata/hypervisor-config-hvc.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("テスト用データベースへの書き込み", func() {
			d, err = db.NewDatabase(cnf.EtcdServerUrl)
			Expect(err).NotTo(HaveOccurred())
			for _, hv := range hvs.Hvs {
				err := d.SetHypervisors(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("移行前データの確認", func() {
			err = d.GetHypervisorsOld(&hvsOld)
			Expect(err).NotTo(HaveOccurred())
			for _, hv := range hvsOld {
				fmt.Println("Node name =", hv.Nodename)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("データ移行実行", func() {
			hvsNew = convertOldToNew(hvsOld)
			for _, hv := range hvsNew {
				fmt.Println("Node name =", hv.Nodename)
				fmt.Println("Port =", hv.Port)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("古いデータの削除", func() {
			err = deleteOldData(hvsOld, d)
			Expect(err).NotTo(HaveOccurred())
		})

		It("新しいデータの登録", func() {
			err = putNewData(hvsNew, d)
			Expect(err).NotTo(HaveOccurred())
		})

		It("移行後データの確認", func() {
			var hvsCheck []types.Hypervisor
			err = d.GetHypervisors(&hvsCheck)
			Expect(err).NotTo(HaveOccurred())
			for _, hv := range hvsCheck {
				fmt.Println("Node name =", hv.Nodename)
				fmt.Println("Port =", hv.Port)
				for _, sp := range hv.StgPool {
					fmt.Println(" Storage Type =", sp.Type)
					fmt.Println(" Storage FreeCap =", sp.FreeCap)
					fmt.Println(" Storage VgCap =", sp.VgCap)
					fmt.Println(" Storage VolGroup =", sp.VolGroup)
				}
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})
