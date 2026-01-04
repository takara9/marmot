package marmotd_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/marmotd"
	ut "github.com/takara9/marmot/pkg/util"
)

var _ = Describe("ボリュームテスト", Ordered, func() {
	const (
		marmotPort        = 8092
		etcdPort          = 7379
		etcdctlExe        = "/usr/bin/etcdctl"
		nodeName          = "hvc"
		etcdImage         = "ghcr.io/takara9/etcd:3.6.5"
		etcdContainerName = "etcd-volume"
	)
	var (
		containerID  string
		ctx          context.Context
		cancel       context.CancelFunc
		marmotServer *marmotd.Server
	)
	etcdUrl := "http://127.0.0.1:" + fmt.Sprintf("%d", etcdPort)

	BeforeAll(func(ctx0 SpecContext) {
	})

	AfterAll(func(ctx0 SpecContext) {
		marmotd.CleanupTestEnvironment()
	})

	Context("テスト環境初期化", func() {
		It("モックサーバー用etcdの起動", func() {
			cmd := exec.Command("docker", "run", "-d", "--name", etcdContainerName, "-p", fmt.Sprintf("%d", etcdPort)+":2379", "-p", fmt.Sprintf("%d", etcdPort+1)+":2380", "--rm", etcdImage)
			output, err := cmd.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
			}
			containerID = string(output[:12]) // 最初の12文字をIDとして取得
			fmt.Printf("Container started with ID: %s\n", containerID)
			time.Sleep(10 * time.Second) // コンテナが起動するまで待機
		})

		It("モックサーバーの起動", func() {
			GinkgoWriter.Println("Start marmot server mock")
			ctx, cancel = context.WithCancel(context.Background())
			marmotServer = marmotd.StartMockServer(ctx, int(marmotPort), int(etcdPort)) // バックグラウンドで起動する
		})

		var hvs config.Hypervisors_yaml
		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc-func.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				fmt.Println(hv)
				err := marmotServer.Ma.Db.SetHypervisors(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := marmotServer.Ma.Db.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のセット", func() {
			for _, sq := range hvs.Seq {
				err := marmotServer.Ma.Db.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("起動完了待ちチェック", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", etcdUrl+"/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("動作確認 CheckHypervisors()", func() {
			GinkgoWriter.Println(nodeName)
			hv, err := marmotServer.Ma.Db.CheckHypervisors(etcdUrl, nodeName)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("xxxxxx array size == ", len(hv))
			for i, v := range hv {
				GinkgoWriter.Println("xxxxxx hv index    == ", i)
				GinkgoWriter.Println("xxxxxx hv nodename == ", v.NodeName)
				GinkgoWriter.Println("xxxxxx hv port     == ", *v.Port)
				GinkgoWriter.Println("xxxxxx hv CPU      == ", v.Cpu)
				GinkgoWriter.Println("xxxxxx hv Mem      == ", *v.Memory)
				GinkgoWriter.Println("xxxxxx hv IP addr  == ", *v.IpAddr)
			}
		})

		It("Check the config file to directly etcd", func() {
			cmd := exec.Command(etcdctlExe, "--endpoints=localhost:7379", "get", "hvc")
			cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
			out, err := cmd.CombinedOutput()
			GinkgoWriter.Println(out)
			Expect(err).To(Succeed()) // 成功
		})
	})

	Context("LV OSボリューム生成から削除", func() {
		var m *marmotd.Marmot
		var volSpec *api.Volume
		var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの生成", func() {
			v := api.Volume{
				Name:      ut.StringPtr("test-os-volume-001"),
				Type:      ut.StringPtr("lvm"),
				Kind:      ut.StringPtr("os"),
				OsVariant: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *volSpec.Key)
		})

		It("OS論理ボリュームの削除", func() {
			err = m.RemoveVolume(volSpec.Id)
			Expect(err).NotTo(HaveOccurred())

			out, err := exec.Command("lvs", "vg1").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリューム リストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("ボリューム のリスト数:", "volume count=", len(vols))
			for i, v := range vols {
				GinkgoWriter.Println("index=", i, "volKey=", *v.Key)
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATA論理ボリューム リストの取得", func() {
			vols, err := m.GetDataVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DATA 論理ボリューム のリスト数:", "volume count=", len(vols))
			for i, v := range vols {
				GinkgoWriter.Println("index=", i, "volKey=", *v.Key)
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの生成 （失敗ケース)", func() {
			v := api.Volume{
				Name:      ut.StringPtr("test-os-volume-001"),
				Type:      ut.StringPtr("lvm"),
				Kind:      ut.StringPtr("os"),
				OsVariant: ut.StringPtr("ubuntu22.NOXIST"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			Expect(err).To(HaveOccurred())
		})

		It("OS論理ボリュームの生成 （失敗ケース)", func() {
			v := api.Volume{
				Name:      ut.StringPtr("test-os-volume-001"),
				Type:      ut.StringPtr("noexist"),
				Kind:      ut.StringPtr("os"),
				OsVariant: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			GinkgoWriter.Println("err=", err)
			Expect(err).To(HaveOccurred())
		})

		It("OS論理ボリュームリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATA論理ボリュームリストの取得", func() {
			vols, err := m.GetDataVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DATA 論理ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>", 0)
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの削除 クリーンナップ", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("ボリュームリストの取得:", "volume count=", len(vols))
			for _, v := range vols {
				if v.Status != nil {
					m.RemoveVolume(*v.Key)
				}
			}

			time.Sleep(1 * time.Second)
			err = m.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("LV DATAボリューム生成から削除", func() {
		var m *marmotd.Marmot
		var volSpec *api.Volume
		var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATA論理ボリュームの生成", func() {
			out, err := exec.Command("lvs", "vg2").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())

			v := api.Volume{
				Name: ut.StringPtr("test-data-volume-001"),
				Type: ut.StringPtr("lvm"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			GinkgoWriter.Println("Creating DATA 論理ボリューム", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *volSpec.Key)
		})

		It("DATA論理ボリュームの削除", func() {
			err = m.RemoveVolume(volSpec.Id)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Marmotインスタンスのクローズ", func() {
			err = m.Close()
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("LV OSとデータのボリューム生成、リスト取得、削除", func() {
		var ids []string
		var m *marmotd.Marmot

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの生成1", func() {
			v := api.Volume{
				Name:      ut.StringPtr("test-os-volume-001"),
				Type:      ut.StringPtr("lvm"),
				Kind:      ut.StringPtr("os"),
				OsVariant: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			ids = append(ids, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("OS論理ボリュームの生成2", func() {
			v := api.Volume{
				Name:      ut.StringPtr("test-os-volume-002"),
				Type:      ut.StringPtr("lvm"),
				Kind:      ut.StringPtr("os"),
				OsVariant: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			ids = append(ids, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("DATA論理ボリュームの生成1", func() {
			v := api.Volume{
				Name: ut.StringPtr("test-data-volume-001"),
				Type: ut.StringPtr("lvm"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			ids = append(ids, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("DATA論理ボリュームの生成2", func() {
			v := api.Volume{
				Name: ut.StringPtr("test-data-volume-002"),
				Type: ut.StringPtr("lvm"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			ids = append(ids, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("OS論理ボリュームリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("論理ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATA論理ボリュームリストの取得", func() {
			vols, err := m.GetDataVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DATA 論理ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("論理ボリュームの削除", func() {
			for _, id := range ids {
				err := m.RemoveVolume(id)
				Expect(err).NotTo(HaveOccurred())
			}
			time.Sleep(1 * time.Second)
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの削除後のリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("論理ボリューム のリスト数:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATA論理ボリュームリストの取得", func() {
			vols, err := m.GetDataVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("DATA 論理ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Marmotインスタンスのクローズ", func() {
			err := m.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("qcow2 OSボリュームの操作", func() {
		var ids []string
		var m *marmotd.Marmot

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))
			Expect(err).NotTo(HaveOccurred())
		})

		It("qcow2ボリュームリストの取得（生成前）", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("生成前qcow2ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
		})

		It("qcow2ボリュームの生成", func() {
			v := api.Volume{
				Name:      ut.StringPtr("test-qcow2-volume-001"),
				Type:      ut.StringPtr("qcow2"),
				Kind:      ut.StringPtr("os"),
				OsVariant: ut.StringPtr("ubuntu22.04"),
			}
			tmpSSpec, err := m.CreateNewVolume(v)
			ids = append(ids, tmpSSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSSpec.Key)
		})

		It("qcow2ボリュームリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("qcow2ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("ls", "-al", "/var/lib/marmot/volumes").Output()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("lvs output:\n", string(out))
		})

		It("qcow2ボリュームの削除", func() {
			for _, id := range ids {
				GinkgoWriter.Println("Removing qcow2 volume", "id", id)
				err := m.RemoveVolume(id)
				Expect(err).NotTo(HaveOccurred())
			}
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Println("削除後のqcow2ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "id=", v.Id, "status=<nil>")
				}
			}
			out, err := exec.Command("ls", "-al", "/var/lib/marmot/volumes").Output()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("lvs output:\n", string(out))
		})

		It("Marmotインスタンスのクローズ", func() {
			err := m.Close()
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("qcow2 DATAボリュームの操作", func() {
		var ids []string
		var m *marmotd.Marmot

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot("hvc", "http://127.0.0.1:"+fmt.Sprintf("%d", etcdPort))
			Expect(err).NotTo(HaveOccurred())
		})

		It("データボリュームの生成と削除", func() {
			v := api.Volume{
				Name: ut.StringPtr("test-qcow2-volume-003"),
				Type: ut.StringPtr("qcow2"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			tmpSpec, err := m.CreateNewVolume(v)
			ids = append(ids, tmpSpec.Id)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume id: ", tmpSpec.Id)

			out, err := exec.Command("ls", "-alh", "/var/lib/marmot/volumes").Output()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("lvs output:\n", string(out))

		})

		It("データボリュームリストの取得", func() {
			vols, err := m.GetDataVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("データボリュームリストの取得:", "volume count=", len(vols))

			out, err := exec.Command("ls", "-alh", "/var/lib/marmot/volumes").Output()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("lvs output:\n", string(out))
		})

		It("データボリュームの削除", func() {
			for _, id := range ids {
				GinkgoWriter.Println("Removing qcow2 volume", "id", id)
				err := m.RemoveVolume(id)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("データボリュームリストの取得 削除後", func() {
			vols, err := m.GetDataVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("データボリュームリストの取得:", "volume count=", len(vols))

			out, err := exec.Command("ls", "-alh", "/var/lib/marmot/volumes").Output()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("lvs output:\n", string(out))
		})

		It("Marmotインスタンスのクローズ", func() {
			err := m.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("コンテナとモックの停止", func() {
		It("停止コマンド実行", func() {
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
