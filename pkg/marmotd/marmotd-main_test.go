package marmotd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("関数テスト", Ordered, func() {
	const (
		marmotPort = 8080
		etcdPort   = 3379
		etcdctlExe = "/usr/bin/etcdctl"
		nodeName   = "hvc"
		etcdImage  = "ghcr.io/takara9/etcd:3.6.5"
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
	)
	//etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)
	marmotEp := "localhost:" + fmt.Sprintf("%d", marmotPort)

	BeforeAll(func(ctx0 SpecContext) {
	})
	AfterAll(func(ctx0 SpecContext) {
		marmotd.CleanupTestEnvironment()
	})

	Context("テスト環境初期化", func() {
		var hvs config.Hypervisors_yaml
		var marmotClient *client.MarmotEndpoint

		It("モックサーバー用etcdの起動", func() {
			cmd := exec.Command("docker", "run", "-d", "--name", "etcd-volume", "-p", fmt.Sprintf("%d", etcdPort)+":2379", "-p", fmt.Sprintf("%d", etcdPort+1)+":2380", "--rm", etcdImage)
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
			}
			containerID = string(output[:12]) // 最初の12文字をIDとして取得
			fmt.Printf("Container started with ID: %s\n", containerID)
			time.Sleep(10 * time.Second) // コンテナが起動するまで待機
		})

		It("モックサーバーの起動", func() {
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
		})

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc-main.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Marmotエンドポイントの生成", func() {
			var err error
			marmotClient, err = client.NewMarmotdEp(
				"http",
				marmotEp,
				"/api/v1",
				60,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		/*
			It("ハイパーバイザーの情報セット", func() {
				for _, hv := range hvs.Hvs {
					fmt.Println(hv)
					err := marmotServer.Ma.Db.SetHypervisors(hv)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		*/

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := marmotServer.Ma.Db.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("シーケンス番号のリセット", func() {
			for _, sq := range hvs.Seq {
				err := marmotServer.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Marmotd の生存確認", func() {
			httpStatus, body, url, err := marmotClient.Ping()
			var replyMessage api.ReplyMessage
			Expect(err).NotTo(HaveOccurred())
			Expect(httpStatus).To(Equal(200))
			err = json.Unmarshal(body, &replyMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(replyMessage.Message).To(Equal("ok"))
			Expect(url).To(BeNil())
		})

		It("Marmotd のバージョン情報取得", func() {
			serverVer, err := marmotClient.GetVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintln(string(*serverVer.ServerVersion))).To(Equal(fmt.Sprintln(marmotd.Version)))
			GinkgoWriter.Println("Version : ", string(*serverVer.ServerVersion))
		})

		var replyVolume api.Volume
		It("DATAボリューム(qcow2)の作成", func() {
			var vol api.Volume
			vol.Name = util.StringPtr("test-volume-001")
			vol.Type = util.StringPtr("qcow2")
			vol.Kind = util.StringPtr("data")
			vol.Size = util.IntPtrInt(100)

			body, url, err := marmotClient.CreateVolume(vol)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &replyVolume)
			GinkgoWriter.Println("CreateVolume replyVolume id = ", replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())
		})

		It("DATAボリューム(qcow2)のリスト取得", func() {
			body, url, err := marmotClient.ListVolumes()
			var vols []api.Volume
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &vols)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vols)).To(BeNumerically(">", 0))
			GinkgoWriter.Println("ListVolumes =", vols)
			Expect(url).To(BeNil())

			out, err := exec.Command("ls", "-alhg", "/var/lib/marmot/volumes").Output()
			GinkgoWriter.Println("ls output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATAボリューム(qcow2)の削除", func() {
			GinkgoWriter.Println("受信したボリューム Id = ", replyVolume.Id)
			body, url, err := marmotClient.DeleteVolumeById(replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DeleteVolumeById body =", string(body))
			Expect(url).To(BeNil())

			out, err := exec.Command("ls", "-alhg", "/var/lib/marmot/volumes").Output()
			GinkgoWriter.Println("ls output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OSボリューム(qcow2)の作成", func() {
			var vol api.Volume
			vol.Name = util.StringPtr("test-volume-002")
			vol.Type = util.StringPtr("qcow2")
			vol.Kind = util.StringPtr("os")
			vol.OsVariant = util.StringPtr("ubuntu22.04")

			body, url, err := marmotClient.CreateVolume(vol)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &replyVolume)
			GinkgoWriter.Println("CreateVolume replyVolume Id = ", replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())

			out, err := exec.Command("ls", "-alhg", "/var/lib/marmot/volumes").Output()
			GinkgoWriter.Println("ls output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OSボリューム(qcow2)の削除", func() {
			GinkgoWriter.Println("受信したボリューム Id = ", replyVolume.Id)
			body, url, err := marmotClient.DeleteVolumeById(replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DeleteVolumeById body =", string(body))
			Expect(url).To(BeNil())
		})

		It("OSボリューム(qcow2)のリスト取得", func() {
			body, url, err := marmotClient.ListVolumes()
			var vols []api.Volume
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &vols)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vols)).To(Equal(0))
			GinkgoWriter.Println("ListVolumes =", vols)
			Expect(url).To(BeNil())

			out, err := exec.Command("ls", "-alhg", "/var/lib/marmot/volumes").Output()
			GinkgoWriter.Println("ls output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OSボリューム(LVM)の作成", func() {
			var vol api.Volume
			vol.Name = util.StringPtr("test-volume-002")
			vol.Type = util.StringPtr("lvm")
			vol.Kind = util.StringPtr("os")
			vol.OsVariant = util.StringPtr("ubuntu22.04")

			body, url, err := marmotClient.CreateVolume(vol)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &replyVolume)
			GinkgoWriter.Println("CreateVolume replyVolume Id = ", replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())
		})

		It("OSボリューム(LVM)のリスト取得 生成後", func() {
			body, url, err := marmotClient.ListVolumes()
			var vols []api.Volume
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &vols)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vols)).To(BeNumerically(">", 0))
			GinkgoWriter.Println("ListVolumes =", vols)
			Expect(url).To(BeNil())

			out, err := exec.Command("lvs", "vg1").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OSボリューム(LVM)の削除", func() {
			GinkgoWriter.Println("受信したボリューム Id = ", replyVolume.Id)
			body, url, err := marmotClient.DeleteVolumeById(replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DeleteVolumeById body =", string(body))
			Expect(url).To(BeNil())
		})

		It("OSボリューム(LVM)のリスト取得 削除後", func() {
			body, url, err := marmotClient.ListVolumes()
			var vols []api.Volume
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &vols)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vols)).To(Equal(0))
			GinkgoWriter.Println("ListVolumes =", vols)
			Expect(url).To(BeNil())

			out, err := exec.Command("lvs", "vg1").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATAボリューム(LVM)の作成 0000", func() {
			var vol api.Volume
			vol.Name = util.StringPtr("test-volume-002")
			vol.Type = util.StringPtr("lvm")
			vol.Kind = util.StringPtr("data")
			vol.Size = util.IntPtrInt(1)

			body, url, err := marmotClient.CreateVolume(vol)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &replyVolume)
			GinkgoWriter.Println("CreateVolume replyVolume Id = ", replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeNil())

		})

		It("DATAボリューム(LVM)のリスト取得 生成後", func() {
			body, url, err := marmotClient.ListVolumes()
			var vols []api.Volume
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &vols)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vols)).To(BeNumerically(">", 0))
			GinkgoWriter.Println("ListVolumes =", vols)
			Expect(url).To(BeNil())

			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATAボリューム(LVM)の詳細取得 0001", func() {
			body, url, err := marmotClient.ShowVolumeById(replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			var vol api.Volume
			err = json.Unmarshal(body, &vol)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("ShowVolumeById Id  =", vol.Id)
			GinkgoWriter.Println("ShowVolumeById Key =", *vol.Key)
			GinkgoWriter.Println("ShowVolumeById VolumeName =", vol.Name)
			Expect(url).To(BeNil())
		})

		It("DATAボリューム(LVM)の情報更新", func() {
			var spec api.Volume
			spec.Name = util.StringPtr("updated-volume-name")
			body, url, err := marmotClient.UpdateVolumeById(replyVolume.Id, spec)
			GinkgoWriter.Println("UpdateVolumeById err =", err)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("UpdateVolumeById body =", string(body))
			Expect(url).To(BeNil())
		})

		It("DATAボリューム(LVM)の詳細取得 0002", func() {
			body, url, err := marmotClient.ShowVolumeById(replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			var vol api.Volume
			err = json.Unmarshal(body, &vol)
			GinkgoWriter.Println("ShowVolumeById Id  =", vol.Id)
			GinkgoWriter.Println("ShowVolumeById Key =", *vol.Key)
			GinkgoWriter.Println("ShowVolumeById VolumeName =", *vol.Name)
			Expect(*vol.Name).To(Equal("updated-volume-name"))
			Expect(url).To(BeNil())
		})

		It("DATAボリューム(LVM)の削除", func() {
			GinkgoWriter.Println("受信したボリューム Id = ", replyVolume.Id)
			body, url, err := marmotClient.DeleteVolumeById(replyVolume.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DeleteVolumeById body =", string(body))
			Expect(url).To(BeNil())
		})

		It("DATAボリューム(LVM)のリスト取得 削除後", func() {
			body, url, err := marmotClient.ListVolumes()
			var vols []api.Volume
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(body, &vols)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(vols)).To(Equal(0))
			GinkgoWriter.Println("ListVolumes =", vols)
			Expect(url).To(BeNil())

			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("停止", func() {
		It("コンテナとモック", func() {
			cmd := exec.Command("docker", "kill", containerID)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to stop container: %v\n", err)
			}
			cmd = exec.Command("docker", "rm", containerID)
			_, err = cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Failed to remove container: %v\n", err)
			}
			cancel() // モックサーバー停止
		})
	})
})
