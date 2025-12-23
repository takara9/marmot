package db_test

import (
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

		/*
			// Setup slog
			opts := &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}
			logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
			slog.SetDefault(logger)
		*/

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

	BeforeEach(func() {
	})

	AfterEach(func() {
	})

	Describe("ボリューム管理テスト", func() {
		var v *db.VolumeController
		Context("基本アクセス", func() {
			var volSpec *api.Volume
			var err error
			It("データボリュームコントローラの生成", func() {
				v, err = db.NewVolumeController(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ボリュームの作成 #1", func() {
				volSpec, err = v.CreateVolumeOnDB("data01", "/var/lib/marmot/volumes/data01.qcow2", "qcow2", "data", 10)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", *volSpec.Key)
			})

			It("Keyからボリューム情報を取得", func() {
				vol, err := v.GetVolumeByKey(*volSpec.Key)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Retrieved volume: Id=%s Key=%s Name=%s Path=%s Size=%d Status=%v\n", vol.Id, *vol.Key, vol.Name, *vol.Path, *vol.Size, db.VolStatus[*vol.Status])
			})

			It("ボリュームの状態更新 #1", func() {
				vol := api.Volume{
					Key:    volSpec.Key,
					Status: util.IntPtrInt(db.VOLUME_AVAILABLE),
				}
				err = v.UpdateVolume(*volSpec.Key, vol)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Keyからボリューム情報を取得", func() {
				vol, err := v.GetVolumeByKey(*volSpec.Key)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Retrieved volume: Id=%s Key=%s Name=%s Path=%s Size=%d Status=%v\n", vol.Id, *vol.Key, vol.Name, *vol.Path, *vol.Size, db.VolStatus[*vol.Status])
			})

			It("ボリュームの作成 #2", func() {
				key, err := v.CreateVolumeOnDB("data02", "/var/lib/marmot/volumes/data02.qcow2", "qcow2", "data", 10)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", key)
			})

			It("ボリュームの作成 #3", func() {
				key, err := v.CreateVolumeOnDB("data03", "/dev/mapper/vg2/datalv0100", "lvm", "data", 10)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", key)
			})

			It("ボリュームの一覧取得", func() {
				vols, err := v.ListVolumes("data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(3))
				fmt.Println("データボリューム一覧:")
				for _, vol := range vols {
					fmt.Printf("Id=%s Key=%s Name=%s Path=%s Size=%d Status=%v\n", vol.Id, *vol.Key, vol.Name, *vol.Path, *vol.Size, db.VolStatus[*vol.Status])
				}
			})

			It("ボリュームの削除", func() {
				vols, err := v.FindVolumeByName("data01", "data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(1))
				err = v.DeleteVolume(*vols[0].Key)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ボリュームの一覧取得", func() {
				vols, err := v.ListVolumes("data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(2))
				fmt.Println("データボリューム一覧:")
				for _, vol := range vols {
					fmt.Printf("Id=%s Key=%s Name=%s Path=%s Size=%d Status=%v\n", vol.Id, *vol.Key, vol.Name, *vol.Path, *vol.Size, db.VolStatus[*vol.Status])
				}
			})
		})
	})
})
