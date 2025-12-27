package marmotd_test

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
	ut "github.com/takara9/marmot/pkg/util"
)

var etcdContainerIdVol string

func prepareMockVolume() {
	fmt.Println("モックサーバーの起動 for ボリュームテスト")
	// Dockerコンテナでetcdサーバーを起動
	cmd := exec.Command("docker", "run", "-d", "--name", "etcdvolume", "-p", "7379:2379", "-p", "7380:2380", "ghcr.io/takara9/etcd:3.6.5")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred())
	etcdContainerIdVol = string(output[:12]) // 最初の12文字をIDとして取得
	fmt.Printf("Container started with ID: %s\n", etcdContainerIdVol)
	time.Sleep(10 * time.Second) // コンテナが起動するまで待機

	//MockServer バックグラウンドで起動する
	e := echo.New()
	server := marmotd.NewServer("hvc", etcdUrlTest)
	go func() {

		/*
			// Setup slog
			opts := &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}
			logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
			slog.SetDefault(logger)
		*/
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("127.0.0.1:8092"), "Mock server is running")
	}()
}

func cleanupMockVolume() {
	fmt.Println("ボリュームテスト用モックサーバーの終了")
	// Dockerコンテナを停止・削除
	cmd := exec.Command("docker", "stop", etcdContainerIdVol)
	_, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to stop container: %v\n", err)
	}
	cmd = exec.Command("docker", "rm", etcdContainerIdVol)
	_, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to remove container: %v\n", err)
	}

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

func testMarmotVolumes() {
	Context("テストデータの初期化", func() {
		var e *db.Database
		It("Set up databae ", func() {
			var err error
			e, err = db.NewDatabase(etcdUrlTest)
			Expect(err).NotTo(HaveOccurred())
		})

		var hvs config.Hypervisors_yaml
		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err := config.ReadYAML("testdata/hypervisor-config-hvc-func.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				err := e.SetHypervisors(hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := e.SetImageTemplate(hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のセット", func() {
			for _, sq := range hvs.Seq {
				err := e.CreateSeq(sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Check up Marmot daemon", func() {
			By("Trying to connect to marmot")
			Eventually(func(g Gomega) {
				cmd := exec.Command("curl", "http://localhost:8092/ping")
				err := cmd.Run()
				GinkgoWriter.Println(cmd, "err= ", err)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())
		})

		It("Check Hypervisors data", func() {
			GinkgoWriter.Println(*nodeNamePtr)
			hv, err := e.CheckHypervisors()
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
			cmd := exec.Command(etcdctl_exe, "--endpoints=localhost:7379", "get", "hvc")
			cmd.Env = append(os.Environ(), "ETCDCTL_API=3")
			out, err := cmd.CombinedOutput()
			GinkgoWriter.Println(out)
			Expect(err).To(Succeed()) // 成功
		})

		It("Close databae ", func() {
			err := e.Close()
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("OSLVの生成から削除", func() {
		var m *marmotd.Marmot
		var volSpec *api.Volume
		var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの生成", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("lvm"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *volSpec.Key)
		})

		// ここが失敗している模様 !!!
		It("OS論理ボリュームの削除", func() {
			fmt.Println("================================!!!!!!!!!!!!!!")
			err = m.RemoveVolume(*volSpec.Key)
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
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("lvm"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.NOXIST"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			Expect(err).To(HaveOccurred())
		})

		It("OS論理ボリュームの生成 （失敗ケース)", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("noexist"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			volSpec, err = m.CreateNewVolume(v)
			GinkgoWriter.Println("err=", err)
			Expect(err).To(HaveOccurred())
		})

		/*
			It("OS 論理ボリュームの生成 （失敗ケース)", func() {
				v := api.Volume{
					Name:   "test-os-volume-001",
					Type:   ut.StringPtr("qcow2"),
					Kind:   ut.StringPtr("os"),
					OsName: ut.StringPtr("ubuntu22.04"),
				}
				GinkgoWriter.Println("Creating OS volume", "volume", v)
				volKey, err = m.CreateNewVolume(v)
				GinkgoWriter.Println("err=", err)
				GinkgoWriter.Println("Created volume key: ", volKey)
				Expect(err).To(HaveOccurred())
			})
		*/

		It("OS論理ボリュームリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
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
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
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

	Context("DATALVの生成から削除", func() {
		var m *marmotd.Marmot
		var volSpec *api.Volume
		var err error

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("DATA論理ボリュームの生成", func() {
			out, err := exec.Command("lvs", "vg2").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())

			v := api.Volume{
				Name: "test-data-volume-001",
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
			err = m.RemoveVolume(*volSpec.Key)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Marmotインスタンスのクローズ", func() {
			err = m.Close()
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("OSとデータの論理ボリューム生成、リスト取得、削除", func() {
		var key []string
		var m *marmotd.Marmot

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("OS論理ボリュームの生成1", func() {
			v := api.Volume{
				Name:   "test-os-volume-001",
				Type:   ut.StringPtr("lvm"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			key = append(key, *tmpSpec.Key)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("OS論理ボリュームの生成2", func() {
			v := api.Volume{
				Name:   "test-os-volume-002",
				Type:   ut.StringPtr("lvm"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			GinkgoWriter.Println("Creating OS volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			key = append(key, *tmpSpec.Key)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("DATA論理ボリュームの生成1", func() {
			v := api.Volume{
				Name: "test-data-volume-001",
				Type: ut.StringPtr("lvm"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			key = append(key, *tmpSpec.Key)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("DATA論理ボリュームの生成2", func() {
			v := api.Volume{
				Name: "test-data-volume-002",
				Type: ut.StringPtr("lvm"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			GinkgoWriter.Println("Creating Data volume", "volume", v)
			tmpSpec, err := m.CreateNewVolume(v)
			key = append(key, *tmpSpec.Key)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)
		})

		It("OS論理ボリュームリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("論理ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
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
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
				}
			}
			out, err := exec.Command("lvs").Output()
			GinkgoWriter.Println("lvs output:\n", string(out))
			Expect(err).NotTo(HaveOccurred())
		})

		It("論理ボリュームの削除", func() {
			for _, k := range key {
				err := m.RemoveVolume(k)
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
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
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
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
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

	Context("OSボリュームの操作", func() {
		var key []string
		var m *marmotd.Marmot

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("qcow2ボリュームリストの取得（生成前）", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("生成前qcow2ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
				}
			}
		})

		It("qcow2ボリュームの生成", func() {
			v := api.Volume{
				Name:   "test-qcow2-volume-001",
				Type:   ut.StringPtr("qcow2"),
				Kind:   ut.StringPtr("os"),
				OsName: ut.StringPtr("ubuntu22.04"),
			}
			//			GinkgoWriter.Println("Creating qcow2 volume", "volume", v.Path)
			tmpSSpec, err := m.CreateNewVolume(v)
			key = append(key, *tmpSSpec.Key)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSSpec.Key)
		})

		It("qcow2ボリュームリストの取得", func() {
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("qcow2ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
				}
			}
			out, err := exec.Command("ls", "-al", "/var/lib/marmot/volumes").Output()
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("lvs output:\n", string(out))
		})

		It("qcow2ボリュームの削除", func() {
			for _, k := range key {
				GinkgoWriter.Println("Removing qcow2 volume", "volKey", k)
				err := m.RemoveVolume(k)
				Expect(err).NotTo(HaveOccurred())
			}
			vols, err := m.GetOsVolumes()
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Println("削除後のqcow2ボリュームリストの取得:", "volume count=", len(vols))
			for i, v := range vols {
				if v.Status != nil {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=", *v.Status)
				} else {
					GinkgoWriter.Println("index=", i, "volKey=", *v.Key, "Name", util.OrDefault(&v.Name, "Null"), "status=<nil>")
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

	Context("DATAボリュームの操作", func() {
		var key []string
		var m *marmotd.Marmot

		It("Marmotインスタンスの生成", func() {
			var err error
			m, err = marmotd.NewMarmot(*nodeNamePtr, *etcdTest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("データボリュームの生成と削除", func() {
			v := api.Volume{
				Name: "test-qcow2-volume-003",
				Type: ut.StringPtr("qcow2"),
				Kind: ut.StringPtr("data"),
				Size: ut.IntPtrInt(1),
			}
			tmpSpec, err := m.CreateNewVolume(v)
			key = append(key, *tmpSpec.Key)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Println("Created volume key: ", *tmpSpec.Key)

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
			for _, k := range key {
				GinkgoWriter.Println("Removing qcow2 volume", "volKey", k)
				err := m.RemoveVolume(k)
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
}
