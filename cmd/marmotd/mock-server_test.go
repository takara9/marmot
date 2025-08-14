package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	//"github.com/takara9/marmot/pkg/cmd"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var _ = Describe("Mock Test", Ordered, func() {
	var ep *MarmotEndpoint
	var err error
	var containerID string
	var Conn *clientv3.Client

	BeforeAll(func(ctx SpecContext) {
		GinkgoWriter.Println("Start marmot server mock")
		startMockServer() // 戻り値なし
		time.Sleep(5 * time.Second)

		// 起動チェック
		ep, err = NewMarmotdEp(
			"http",
			"127.0.0.1:8080",
			"/api/v1",
			10)
		if err != nil {
			GinkgoWriter.Println("Error creating MarmotEndpoint:", err)
		} else {
			GinkgoWriter.Println("MarmotEndpoint created successfully:", ep)
		}

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "etcd0", "-p", "12379:2379", "-p", "12380:2380", "-e", "ALLOW_NONE_AUTHENTICATION=yes", "-e", "ETCD_ADVERTISE_CLIENT_URLS=http://127.0.0.1:12379", "bitnami/etcd")
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

	Context("基本的なアクセステスト", func() {
		const hypervior_config string = "testdata/hypervisor-config.yaml"
		var hvs config.Hypervisors_yaml

		It("ハイパーバイザーのコンフィグファイルの読み取り", func() {
			err = config.ReadYAML("testdata/hypervisor-config.yaml", &hvs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("データベースへの接続", func() {
			Conn, err = db.Connect("http://127.0.0.1:12379")
			Expect(err).NotTo(HaveOccurred())
		})

		It("ハイパーバイザーの情報セット", func() {
			for _, hv := range hvs.Hvs {
				fmt.Println(hv)
				err := db.SetHypervisor(Conn, hv)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("OSイメージテンプレート", func() {
			for _, hd := range hvs.Imgs {
				err := db.SetImageTemplate(Conn, hd)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("シーケンス番号のリセット", func() {
			for _, sq := range hvs.Seq {
				err := db.CreateSeq(Conn, sq.Key, sq.Start, sq.Step)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Marmotd の EPを確認", func() {
			GinkgoWriter.Println("epが作成され nil でないこと ep:", ep)
			Expect(ep).ToNot(BeNil(), "作成に失敗している。")
		})

		It("Marmotd の生存確認", func() {
			statusCode, body, url, err := ep.Ping()
			GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
			Expect(err).To(BeNil(), "Expected no error from ping")
			Expect(statusCode).To(Equal(200), "Expected status code 200 from ping")
			Expect(string(body)).To(Equal("{\"message\":\"ok\"}\n"), "Expected body to be 'pong'")
			Expect(url).To(BeNil(), "Expected no URL from ping response")
		})

		It("Marmotd のバージョンを取得できること", func() {
			statusCode, body, url, err := ep.GetVersion()
			GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
			Expect(err).To(BeNil(), "Expected no error")
			Expect(statusCode).To(Equal(200), "Expected status code 200")
			Expect(string(body)).To(Equal("{\"version\":\"0.0.1\"}\n"), "Expected body to be 'pong'")
			Expect(url).To(BeNil(), "Expected no URL from ping response")
		})

		It("テスト用データベースでVGの確認ができること", func() {
			var node *string
			var etcd *string
			a := "hvc"
			node = &a
			b := "http://127.0.0.1:12379"
			etcd = &b
			err := util.CheckHvVgAll(*etcd, *node)
			Expect(err).To(BeNil(), "Expected no error")
		})

		It("管理下のハイパーバイザーがリストされること", func() {
			statusCode, body, url, err := ep.ListHypervisors()
			GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
			Expect(err).To(BeNil(), "Expected no error")
			Expect(statusCode).To(Equal(200), "Expected status code")
			var hypervisors api.Hypervisors
			err = json.Unmarshal(body, &hypervisors)
			Expect(err).To(BeNil(), "Expected no error unmarshalling hypervisors")
			Expect(len(hypervisors)).To(BeNumerically(">", 0), "Expected at least one hypervisor")
			for _, hv := range hypervisors {
				GinkgoWriter.Printf("Hypervisor: %+v", hv.NodeName)
				GinkgoWriter.Printf("    cpu: %+v", hv.Cpu)
				if hv.IpAddr != nil {
					GinkgoWriter.Printf("    IP:  %+v", *hv.IpAddr)
				}
				if hv.Memory != nil {
					GinkgoWriter.Printf("    Mem: %+v", *hv.Memory)
				}
				GinkgoWriter.Println()
			}
		})

		It("仮想マシンクラスタの生成", func() {
			// コンフィグファイルを読んで、marmotdに送信する。
			var cnf config.MarmotConfig
			err := config.ReadConfig("testdata/cluster-config.yaml", &cnf)
			Expect(err).To(BeNil())


			// ここで、構造体の変換を実施して、リクエストを送信する

			_,_, err = ReqRest(cnf, "createCluster", "http://localhost:8080")
			Expect(err).To(BeNil())
			GinkgoWriter.Println(err.Error())	
		})

		It("仮想マシンの生成", func() {
			GinkgoWriter.Println("これからテストするのは、断念して、create_clusterから先に実施する")
			//Conn, err := db.Connect("http://127.0.0.1:12379")
			// VMクラスタの生成からテストを実施した方が良さそう。
			/*
				name := "test_vm1"
				var cpu int32 = 1
				var memory int64 = 1024
				privateIP := "172.16.0.250"

				vmKey, err := db.FindByHostAndClusteName(Conn, name, "dev")
				Expect(err).NotTo(HaveOccurred())

				var ostvg string = "vg1"
				var ostlv string = "lv01"
				var osvar string = "ubuntu20.04"
				//comment := "test VM for CI"
				var x []api.Storage

				name0 := "Data1"
				size0 := int64(200)
				vg0 := "vg2"
				x0 := api.Storage{Name: &name0, Size: &size0, Vg: &vg0}
				x = append(x, x0)


				name1 := "Data2"
				size1 := int64(200)
				vg1 := "vg2"
				x2 := api.Storage{Name: &name1, Size: &size1, Vg: &vg1}
				x = append(x, x2)
				vm := api.VmSpec{
					Key: 			 &vmKey,
					Name:      &name,
					Ostempvariant: &osvar,
					Cpu:       &cpu,
					Memory:    &memory,
					PrivateIp: &privateIP,
					Storage:   &x,
					Ostempvg:  &ostvg,
					Ostemplv:  &ostlv,
				}
				statusCode, body, url, err := ep.CreateVirtualMachine(vm)
				GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
				Expect(err).To(BeNil(), "Expected no error")
				Expect(statusCode).To(Equal(200), "Expected status code")
			*/
		})
	})

	It("管理下の仮想マシンがリストされること、仮想サーバーが作られてないので ", func() {
		statusCode, body, url, err := ep.ListVirtualMachines()
		GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
		Expect(err).To(BeNil(), "Expected no error")
		Expect(statusCode).To(Equal(200), "Expected status code")
	})

})
