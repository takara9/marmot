package marmotd_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

// var err error
var marmotServerTest *marmotd.Server
var containerID string

func prepareMockServers() {
	// Etcdサーバーのモック起動
	GinkgoWriter.Println("etcdのモック起動")
	cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "3379:2379", "-p", "3380:2380", "ghcr.io/takara9/etcd:3.6.5")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred())
	containerID = string(output[:12]) // 最初の12文字をIDとして取得
	fmt.Printf("Container started with ID: %s\n", containerID)
	time.Sleep(10 * time.Second) // コンテナが起動するまで待機

	// Marmotサーバーのモック起動
	GinkgoWriter.Println("Marmotサーバーのモック起動")
	e := echo.New()
	marmotServerTest = marmotd.NewServer("hvc", "http://127.0.0.1:3379")
	go func() {
		// Setup slog
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		api.RegisterHandlersWithBaseURL(e, marmotServerTest, "/api/v1")
		fmt.Println(e.Start("127.0.0.1:8080"), "Mock server is running")
	}()
}

func cleanupMockServers() {
	fmt.Println("モックサーバーのクリーンナップ")
	cmd := exec.Command("docker", "stop", containerID)
	_, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred())
	cmd = exec.Command("docker", "rm", containerID)
	_, err = cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred())

	cmd = exec.Command("lvremove vg1/oslv0900 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg1/oslv0901 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg1/oslv0902 -y")
	cmd.CombinedOutput()

	cmd = exec.Command("lvremove vg2/data0900 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0901 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0902 -y")
	cmd.CombinedOutput()
	cmd = exec.Command("lvremove vg2/data0903 -y")
	cmd.CombinedOutput()

	cmd = exec.Command("docker kill $(docker ps |awk 'NR>1 {print $1}')")
	cmd.CombinedOutput()

	cmd = exec.Command("docker rm $(docker ps --all |awk 'NR>1 {print $1}')")
	cmd.CombinedOutput()
}

func testMarmotd() {
	var hvs config.Hypervisors_yaml
	var marmotClient *client.MarmotEndpoint

	It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
		err := config.ReadYAML("testdata/hypervisor-config-hvc-main.yaml", &hvs)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Marmotエンドポイントの生成", func() {
		var err error
		marmotClient, err = client.NewMarmotdEp(
			"http",
			"localhost:8080",
			"/api/v1",
			60,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("ハイパーバイザーの情報セット", func() {
		for _, hv := range hvs.Hvs {
			fmt.Println(hv)
			err := marmotServerTest.Ma.Db.SetHypervisors(hv)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("OSイメージテンプレート", func() {
		for _, hd := range hvs.Imgs {
			err := marmotServerTest.Ma.Db.SetImageTemplate(hd)
			Expect(err).NotTo(HaveOccurred())
		}
	})
	It("シーケンス番号のリセット", func() {
		for _, sq := range hvs.Seq {
			err := marmotServerTest.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
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

	It("ハイパーバイザーの一覧取得", func() {
		httpStatus, body, url, err := marmotClient.ListHypervisors(nil)
		var hvs []api.Hypervisor
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		err = json.Unmarshal(body, &hvs)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(hvs)).To(Equal(1))
		Expect(hvs[0].NodeName).To(Equal("hvc"))
		Expect(*hvs[0].FreeCpu).To(Equal(int32(4)))
		Expect(url).To(BeNil())
	})

	It("ハイパーバイザーの情報取得", func() {
		httpStatus, body, url, err := marmotClient.GetHypervisor("hvc")
		var hv api.Hypervisor
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(http.StatusOK))
		err = json.Unmarshal(body, &hv)
		Expect(err).NotTo(HaveOccurred())
		Expect(hv.NodeName).To(Equal("hvc"))
		Expect(*hv.FreeCpu).To(Equal(int32(4)))
		Expect(url).To(BeNil())
	})

	It("存在しないハイパーバイザーの情報取得", func() {
		httpStatus, body, url, err := marmotClient.GetHypervisor("hvc-noexist")
		var replyMessage api.ReplyMessage
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(http.StatusNotFound))
		err = json.Unmarshal(body, &replyMessage)
		Expect(err).NotTo(HaveOccurred())
		Expect(replyMessage.Message).To(Equal("Hypervisor hvc-noexist not found"))
		Expect(url).To(BeNil())
	})

	It("クラスタの生成", func() {
		cnf, err := config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
		Expect(err).NotTo(HaveOccurred())
		httpStatus, body, url, err := marmotClient.CreateCluster(*cnf)
		GinkgoWriter.Println("CreateCluster ERR = ", err)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		var replyMessage api.ReplyMessage
		err = json.Unmarshal(body, &replyMessage)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
	})

	It("仮想マシンの一覧取得", func() {
		httpStatus, body, url, err := marmotClient.ListVirtualMachines(nil)
		var vms []api.VirtualMachine
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		err = json.Unmarshal(body, &vms)
		GinkgoWriter.Println("err = ", err)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(vms)).To(Equal(2))
		Expect(url).To(BeNil())
	})

	It("クラスタの一時停止", func() {
		cnf, err := config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
		Expect(err).NotTo(HaveOccurred())
		httpStatus, body, url, err := marmotClient.StopCluster(*cnf)
		GinkgoWriter.Println("StopCluster ERR = ", err)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		var replyMessage api.ReplyMessage
		err = json.Unmarshal(body, &replyMessage)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
		time.Sleep(time.Second * 20)
	})

	It("クラスタの再スタート", func() {
		cnf, err := config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
		Expect(err).NotTo(HaveOccurred())
		httpStatus, body, url, err := marmotClient.StartCluster(*cnf)
		GinkgoWriter.Println("StartCluster ERR = ", err)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		var replyMessage api.ReplyMessage
		err = json.Unmarshal(body, &replyMessage)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
	})

	It("クラスタの削除", func() {
		cnf, err := config.ReadYamlClusterConfig("testdata/cluster-config.yaml")
		Expect(err).NotTo(HaveOccurred())
		httpStatus, body, url, err := marmotClient.DestroyCluster(*cnf)
		GinkgoWriter.Println("DestroyCluster ERR = ", err)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		var replyMessage api.ReplyMessage
		err = json.Unmarshal(body, &replyMessage)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
	})

	var replyVolume api.Volume
	It("DATAボリューム(qcow2)の作成", func() {
		var vol api.Volume
		vol.Name = "test-volume-001"
		vol.Type = util.StringPtr("qcow2")
		vol.Kind = util.StringPtr("data")
		vol.Size = util.IntPtrInt(100)

		httpStatus, body, url, err := marmotClient.CreateVolume(vol)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		err = json.Unmarshal(body, &replyVolume)
		GinkgoWriter.Println("CreateVolume replyVolume Key = ", *replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
	})

	It("DATAボリューム(qcow2)のリスト取得", func() {
		httpStatus, body, url, err := marmotClient.ListVolumes()
		var vols []api.Volume
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
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
		GinkgoWriter.Println("受信したボリュームキー = ", *replyVolume.Key)
		httpStatus, body, url, err := marmotClient.DeleteVolumeById(*replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		GinkgoWriter.Println("DeleteVolumeById body =", string(body))
		Expect(url).To(BeNil())

		out, err := exec.Command("ls", "-alhg", "/var/lib/marmot/volumes").Output()
		GinkgoWriter.Println("ls output:\n", string(out))
		Expect(err).NotTo(HaveOccurred())
	})

	It("OSボリューム(qcow2)の作成", func() {
		var vol api.Volume
		vol.Name = "test-volume-002"
		vol.Type = util.StringPtr("qcow2")
		vol.Kind = util.StringPtr("os")
		vol.OsName = util.StringPtr("ubuntu22.04")

		httpStatus, body, url, err := marmotClient.CreateVolume(vol)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		err = json.Unmarshal(body, &replyVolume)
		GinkgoWriter.Println("CreateVolume replyVolume Key = ", *replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())

		out, err := exec.Command("ls", "-alhg", "/var/lib/marmot/volumes").Output()
		GinkgoWriter.Println("ls output:\n", string(out))
		Expect(err).NotTo(HaveOccurred())
	})

	It("OSボリューム(qcow2)の削除", func() {
		GinkgoWriter.Println("受信したボリュームキー = ", *replyVolume.Key)
		httpStatus, body, url, err := marmotClient.DeleteVolumeById(*replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		GinkgoWriter.Println("DeleteVolumeById body =", string(body))
		Expect(url).To(BeNil())
	})

	It("OSボリューム(qcow2)のリスト取得", func() {
		httpStatus, body, url, err := marmotClient.ListVolumes()
		var vols []api.Volume
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
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
		vol.Name = "test-volume-002"
		vol.Type = util.StringPtr("lvm")
		vol.Kind = util.StringPtr("os")
		vol.OsName = util.StringPtr("ubuntu22.04")

		httpStatus, body, url, err := marmotClient.CreateVolume(vol)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		err = json.Unmarshal(body, &replyVolume)
		GinkgoWriter.Println("CreateVolume replyVolume Key = ", *replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())
	})

	It("OSボリューム(LVM)のリスト取得 生成後", func() {
		httpStatus, body, url, err := marmotClient.ListVolumes()
		var vols []api.Volume
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
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
		GinkgoWriter.Println("受信したボリュームキー = ", *replyVolume.Key)
		httpStatus, body, url, err := marmotClient.DeleteVolumeById(*replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		GinkgoWriter.Println("DeleteVolumeById body =", string(body))
		Expect(url).To(BeNil())
	})

	It("OSボリューム(LVM)のリスト取得 削除後", func() {
		httpStatus, body, url, err := marmotClient.ListVolumes()
		var vols []api.Volume
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
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
		vol.Name = "test-volume-002"
		vol.Type = util.StringPtr("lvm")
		vol.Kind = util.StringPtr("data")
		vol.Size = util.IntPtrInt(1)

		httpStatus, body, url, err := marmotClient.CreateVolume(vol)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(201))
		err = json.Unmarshal(body, &replyVolume)
		GinkgoWriter.Println("CreateVolume replyVolume Key = ", *replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(url).To(BeNil())

	})

	It("DATAボリューム(LVM)のリスト取得 生成後", func() {
		httpStatus, body, url, err := marmotClient.ListVolumes()
		var vols []api.Volume
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
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
		httpStatus, body, url, err := marmotClient.ShowVolumeById(*replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
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
		spec.Name = "updated-volume-name"
		httpStatus, body, url, err := marmotClient.UpdateVolumeById(*replyVolume.Key, spec)
		GinkgoWriter.Println("UpdateVolumeById err =", err)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		GinkgoWriter.Println("UpdateVolumeById body =", string(body))
		Expect(url).To(BeNil())
	})

	It("DATAボリューム(LVM)の詳細取得 0002", func() {
		httpStatus, body, url, err := marmotClient.ShowVolumeById(*replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		var vol api.Volume
		err = json.Unmarshal(body, &vol)
		//Expect(err).NotTo(HaveOccurred())
		//GinkgoWriter.Println("ShowVolumeById volume =", vol)
		GinkgoWriter.Println("ShowVolumeById Id  =", vol.Id)
		GinkgoWriter.Println("ShowVolumeById Key =", *vol.Key)
		GinkgoWriter.Println("ShowVolumeById VolumeName =", vol.Name)
		Expect(vol.Name).To(Equal("updated-volume-name"))
		Expect(url).To(BeNil())
	})

	It("DATAボリューム(LVM)の削除", func() {
		GinkgoWriter.Println("受信したボリュームキー = ", *replyVolume.Key)
		httpStatus, body, url, err := marmotClient.DeleteVolumeById(*replyVolume.Key)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		GinkgoWriter.Println("DeleteVolumeById body =", string(body))
		Expect(url).To(BeNil())
	})

	It("DATAボリューム(LVM)のリスト取得 削除後", func() {
		httpStatus, body, url, err := marmotClient.ListVolumes()
		var vols []api.Volume
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStatus).To(Equal(200))
		err = json.Unmarshal(body, &vols)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(vols)).To(Equal(0))
		GinkgoWriter.Println("ListVolumes =", vols)
		Expect(url).To(BeNil())

		out, err := exec.Command("lvs").Output()
		GinkgoWriter.Println("lvs output:\n", string(out))
		Expect(err).NotTo(HaveOccurred())
	})
}
