package db_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Image", Ordered, func() {
	var port int = 11379
	var url string = "http://127.0.0.1:" + fmt.Sprintf("%d", port)
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%d:2379", port), "ghcr.io/takara9/etcd:3.6.5")
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
	}, NodeTimeout(20*time.Second))

	Describe("イメージ管理のテスト", func() {
		var v *db.Database
		var id string
		Context("基本アクセス", func() {
			var err error
			It("データベースへの接続", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("イメージの作成 #1", func() {
				url := "http://hmc/ubuntu-22.04-server-cloudimg-amd64.img"
				id, err = v.MakeImageEntryFromURL("test-image-1", url)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created image with ID:", id)
			})

			It("Keyからイメージ情報を取得 #1", func() {
				img, err := v.GetImage(id)
				Expect(err).NotTo(HaveOccurred())
				jsonBytes, err := json.MarshalIndent(img, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(jsonBytes))
			})

			It("イメージの作成 #2", func() {
				url := "http://hmc/Rocky-9-GenericCloud.latest.x86_64.qcow2"
				id, err = v.MakeImageEntryFromURL("test-image-2", url)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created image with ID:", id)
			})

			It("イメージの作成 #3 nodeName付き", func() {
				url := "http://hmc/debian-12-genericcloud-amd64.qcow2"
				id, err = v.MakeImageEntryFromURLWithNode("test-image-3", url, "hv-test-01")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created image with ID:", id)
			})

			It("Keyからイメージ情報を取得 #3 nodeName確認", func() {
				img, err := v.GetImage(id)
				Expect(err).NotTo(HaveOccurred())
				Expect(img.Metadata).NotTo(BeNil())
				Expect(img.Metadata.NodeName).NotTo(BeNil())
				Expect(*img.Metadata.NodeName).To(Equal("hv-test-01"))
			})

			It("Keyからイメージ情報を取得 #2", func() {
				img, err := v.GetImage(id)
				Expect(err).NotTo(HaveOccurred())
				jsonBytes, err := json.MarshalIndent(img, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(jsonBytes))
			})

			ids := make([]string, 10)
			It("すべてのイメージ情報を取得", func() {
				imgs, err := v.GetImages()
				Expect(err).NotTo(HaveOccurred())
				for i, img := range imgs {
					fmt.Println("Id", img.Id, "Name", *img.Metadata.Name, "Status", db.ImageStatus[img.Status.StatusCode])
					ids[i] = img.Id
				}
			})

			It("イメージの削除", func() {
				err := v.DeleteImage(ids[0])
				Expect(err).NotTo(HaveOccurred())
			})

			It("すべてのイメージ情報を取得", func() {
				imgs, err := v.GetImages()
				Expect(err).NotTo(HaveOccurred())
				for i, img := range imgs {
					fmt.Println("Id", img.Id, "Name", *img.Metadata.Name, "Status", db.ImageStatus[img.Status.StatusCode])
					ids[i] = img.Id
				}
			})
		})
	})
})
