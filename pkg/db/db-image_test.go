package db_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

func stringPtr(v string) *string {
	return &v
}

func intPtr(v int) *int {
	return &v
}

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
				id, err = v.MakeImageEntryFromURLWithNode("test-image-1", url, "hv-test-01")
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
				id, err = v.MakeImageEntryFromURLWithNode("test-image-2", url, "hv-test-02")
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

			It("イメージ名とノード名でイメージを取得できる", func() {
				if v == nil {
					var connErr error
					v, connErr = db.NewDatabase(url)
					Expect(connErr).NotTo(HaveOccurred())
				}

				_, err = v.MakeImageEntryFromURLWithNode("test-image-node-aware", "http://hmc/node1.qcow2", "marmot1")
				Expect(err).NotTo(HaveOccurred())
				_, err = v.MakeImageEntryFromURLWithNode("test-image-node-aware", "http://hmc/node2.qcow2", "marmot2")
				Expect(err).NotTo(HaveOccurred())

				img, err := v.FindImageByNameAndNode("test-image-node-aware", "marmot2")
				Expect(err).NotTo(HaveOccurred())
				Expect(img.Metadata).NotTo(BeNil())
				Expect(img.Metadata.NodeName).NotTo(BeNil())
				Expect(*img.Metadata.NodeName).To(Equal("marmot2"))
			})

			It("MakeImageEntryFromSpec が osName/osVersion を保持する", func() {
				if v == nil {
					var connErr error
					v, connErr = db.NewDatabase(url)
					Expect(connErr).NotTo(HaveOccurred())
				}

				req := api.Image{
					ApiVersion: "v1",
					Kind:       "Image",
					Metadata: api.Metadata{
						Name:     "test-image-spec-os",
						NodeName: stringPtr("hv-test-03"),
					},
					Spec: api.ImageSpec{
						SourceUrl: stringPtr("https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img"),
						OsName:    stringPtr("ubuntu"),
						OsVersion: stringPtr("24.04"),
					},
				}

				createdID, createErr := v.MakeImageEntryFromSpec(req)
				Expect(createErr).NotTo(HaveOccurred())

				img, getErr := v.GetImage(createdID)
				Expect(getErr).NotTo(HaveOccurred())
				Expect(img.Spec.SourceUrl).NotTo(BeNil())
				Expect(*img.Spec.SourceUrl).To(Equal("https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img"))
				Expect(img.Spec.OsName).NotTo(BeNil())
				Expect(*img.Spec.OsName).To(Equal("ubuntu"))
				Expect(img.Spec.OsVersion).NotTo(BeNil())
				Expect(*img.Spec.OsVersion).To(Equal("24.04"))
			})

			It("MakeFollowerImageEntry が osName/osVersion/sourceUrl を引き継ぐ", func() {
				if v == nil {
					var connErr error
					v, connErr = db.NewDatabase(url)
					Expect(connErr).NotTo(HaveOccurred())
				}

				head := api.Image{
					ApiVersion: "v1",
					Kind:       "Image",
					Metadata: api.Metadata{
						Id:       "head-image-id",
						Name:     "ubuntu26.04",
						NodeName: stringPtr("marmot1"),
					},
					Spec: api.ImageSpec{
						Kind:      stringPtr("os"),
						Type:      stringPtr("qcow2"),
						SourceUrl: stringPtr("https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img"),
						OsName:    stringPtr("ubuntu"),
						OsVersion: stringPtr("26.04"),
						Size:      intPtr(16),
					},
				}

				followerID, err := v.MakeFollowerImageEntry(head, "marmot2", "head-image-id")
				Expect(err).NotTo(HaveOccurred())

				follower, err := v.GetImage(followerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(follower.Spec.OsName).NotTo(BeNil())
				Expect(*follower.Spec.OsName).To(Equal("ubuntu"))
				Expect(follower.Spec.OsVersion).NotTo(BeNil())
				Expect(*follower.Spec.OsVersion).To(Equal("26.04"))
				Expect(follower.Spec.SourceUrl).NotTo(BeNil())
				Expect(*follower.Spec.SourceUrl).To(Equal("https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img"))
			})

			It("UpdateImageStatus は status.message を nil にクリアする", func() {
				if v == nil {
					var connErr error
					v, connErr = db.NewDatabase(url)
					Expect(connErr).NotTo(HaveOccurred())
				}

				createdID, err := v.MakeImageEntryFromURLWithNode("test-image-status-message", "http://hmc/status-message.qcow2", "marmot1")
				Expect(err).NotTo(HaveOccurred())

				v.UpdateImageStatusMessage(createdID, db.IMAGE_CREATING, "ヘッドノードからQCOW2イメージを取得中")
				creating, err := v.GetImage(createdID)
				Expect(err).NotTo(HaveOccurred())
				Expect(creating.Status).NotTo(BeNil())
				Expect(creating.Status.Message).NotTo(BeNil())
				Expect(*creating.Status.Message).To(Equal("ヘッドノードからQCOW2イメージを取得中"))

				v.UpdateImageStatus(createdID, db.IMAGE_AVAILABLE)
				available, err := v.GetImage(createdID)
				Expect(err).NotTo(HaveOccurred())
				Expect(available.Status).NotTo(BeNil())
				Expect(available.Status.StatusCode).To(Equal(db.IMAGE_AVAILABLE))
				Expect(available.Status.Message).To(BeNil())
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
					fmt.Println("Id", img.Metadata.Id, "Name", img.Metadata.Name, "Status", db.ImageStatus[img.Status.StatusCode])
					ids[i] = img.Metadata.Id
				}
			})

			It("イメージの削除", func() {
				err := v.DeleteImage(ids[0])
				Expect(err).NotTo(HaveOccurred())
			})

			It("イメージの削除予定日時を設定できる", func() {
				localDB, err := db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					Expect(localDB.Close()).NotTo(HaveOccurred())
				}()

				createdID, err := localDB.MakeImageEntryFromURLWithNode("test-image-delete-ts", "http://hmc/delete-ts.qcow2", "hv-test-01")
				Expect(err).NotTo(HaveOccurred())

				img, err := localDB.GetImage(createdID)
				Expect(err).NotTo(HaveOccurred())
				Expect(img.Status).NotTo(BeNil())
				Expect(img.Status.DeletionTimeStamp).To(BeNil())

				err = localDB.SetDeleteTimestampImage(createdID)
				Expect(err).NotTo(HaveOccurred())

				updated, err := localDB.GetImage(createdID)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Status).NotTo(BeNil())
				Expect(updated.Status.DeletionTimeStamp).NotTo(BeNil())
			})

			It("すべてのイメージ情報を取得", func() {
				imgs, err := v.GetImages()
				Expect(err).NotTo(HaveOccurred())
				for i, img := range imgs {
					fmt.Println("Id", img.Metadata.Id, "Name", img.Metadata.Name, "Status", db.ImageStatus[img.Status.StatusCode])
					ids[i] = img.Metadata.Id
				}
			})
		})
	})
})
