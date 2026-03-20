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
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Volumes", Ordered, func() {
	var url string = "http://127.0.0.1:6379"
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "jobEtcdDb", "-p", "6379:2379", "-p", "6380:2380", "ghcr.io/takara9/etcd:3.6.5")
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

	Describe("ボリューム管理テスト", func() {
		var v *db.Database
		var volSpec *api.Volume
		Context("基本アクセス", func() {
			var err error
			It("データボリュームコントローラの生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ボリュームの作成 #1", func() {
				vol := &api.Volume{
					Metadata: &api.Metadata{
						Name: util.StringPtr("data01"),
					},
					Spec: &api.VolSpec{
						Path: util.StringPtr("/var/lib/marmot/volumes/data01.qcow2"),
						Type: util.StringPtr("qcow2"),
						Kind: util.StringPtr("data"),
						Size: util.IntPtrInt(1),
					},
				}
				volSpec, err = v.CreateVolumeOnDB2(*vol)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", volSpec.Id)
			})

			It("Keyからボリューム情報を取得", func() {
				vol, err := v.GetVolumeById(volSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Retrieved volume: Id=%s Key=%s Name=%s Path=%s Size=%d Status=%v\n", vol.Id, *vol.Metadata.Key, *vol.Metadata.Name, *vol.Spec.Path, *vol.Spec.Size, db.VolStatus[*vol.Status.Status])
			})

			It("ボリュームの状態更新 #1", func() {
				vol := api.Volume{
					Status: &api.Status{
						Status: util.IntPtrInt(db.VOLUME_AVAILABLE),
					},
				}
				err = v.UpdateVolume(volSpec.Id, vol)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Keyからボリューム情報を取得", func() {
				vol, err := v.GetVolumeById(volSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				jsonData, err := json.MarshalIndent(vol, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(jsonData))
			})

			It("ボリュームの作成 #2", func() {
				vol := &api.Volume{
					Metadata: &api.Metadata{
						Name: util.StringPtr("data02"),
					},
					Spec: &api.VolSpec{
						Path: util.StringPtr("/var/lib/marmot/volumes/data02.qcow2"),
						Type: util.StringPtr("qcow2"),
						Kind: util.StringPtr("data"),
						Size: util.IntPtrInt(2),
					},
				}
				volSpec, err := v.CreateVolumeOnDB2(*vol)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", volSpec.Id)
			})

			It("ボリュームの作成 #3", func() {
				vol := &api.Volume{
					Metadata: &api.Metadata{
						Name: util.StringPtr("data03"),
					},
					Spec: &api.VolSpec{
						Path: util.StringPtr("/var/lib/marmot/volumes/data03.qcow2"),
						Type: util.StringPtr("qcow2"),
						Kind: util.StringPtr("data"),
						Size: util.IntPtrInt(3),
					},
				}
				volSpec, err := v.CreateVolumeOnDB2(*vol)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", volSpec.Id)
			})

			It("ボリュームの一覧取得", func() {
				vols, err := v.ListVolumes("data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(3))
				fmt.Println("データボリューム一覧:")
				for _, vol := range vols {
					jsonData, err := json.MarshalIndent(vol, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})

			It("ボリュームの削除", func() {
				vols, err := v.FindVolumeByName("data01", "data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(1))
				err = v.DeleteVolume(vols[0].Id)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ボリュームの一覧取得", func() {
				vols, err := v.ListVolumes("data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(2))
				fmt.Println("データボリューム一覧:")
				for _, vol := range vols {
					jsonData, err := json.MarshalIndent(vol, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})
		})
	})
})
