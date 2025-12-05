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

var _ = Describe("Volumes", Ordered, func() {
	var url string = "http://127.0.0.1:6379"
	var err error
	var containerID string
	//var j *db.Job

	BeforeAll(func(ctx SpecContext) {
		// Setup slog
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

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
			It("データボリュームコントローラの生成", func() {
				v, err = db.NewVolumeController(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("データボリュームの作成 #1", func() {
				id, err := v.CreateVolume("data01", "/var/lib/marmot/volumes/data01.qcow2", "qcow2", "data", 10)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", id)
			})

			It("データボリュームの作成 #2", func() {
				id, err := v.CreateVolume("data02", "/var/lib/marmot/volumes/data02.qcow2", "qcow2", "data", 10)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", id)
			})

			It("データボリュームの作成 #3", func() {
				id, err := v.CreateVolume("data03", "/dev/mapper/vg2/datalv0100", "lvm", "data", 10)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created data volume with ID:", id)
			})

			It("データボリュームの一覧取得", func() {
				vols, err := v.ListVolumes("data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(3))
				fmt.Println("データボリューム一覧:")
				for _, vol := range vols {
					fmt.Printf("Id=%s Key=%s Name=%s Path=%s SizeGB=%d\n", vol.Id, vol.Key, vol.VolumeName, vol.Path, vol.SizeGB)
				}
			})

			It("データボリュームの削除", func() {
				vols, err := v.FindVolumeByName("data01", "data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(1))
				err = v.DeleteVolume(vols[0].Key)
				Expect(err).NotTo(HaveOccurred())
			})

			It("データボリュームの一覧取得", func() {
				vols, err := v.ListVolumes("data")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(vols)).To(Equal(2))
				fmt.Println("データボリューム一覧:")
				for _, vol := range vols {
					fmt.Printf("Id=%s Key=%s Name=%s Path=%s SizeGB=%d\n", vol.Id, vol.Key, vol.VolumeName, vol.Path, vol.SizeGB)
				}
			})
		})
	})
})
