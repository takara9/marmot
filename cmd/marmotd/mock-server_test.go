package main

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("Mock Test", Ordered, func() {
	var ep *MarmotEndpoint
	var err error
	BeforeAll(func(ctx SpecContext) {
		GinkgoWriter.Println("Start marmot server mock")
		startMockServer() // 戻り値なし
		time.Sleep(5 * time.Second)
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
	}, NodeTimeout(20*time.Second))

	Context("基本的なアクセステスト", func() {

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

		It("テスト用データベースとの接続ができること", func() {
			var node *string
			var etcd *string
			a := "hv1"
			node = &a
			b := "http://127.0.0.1:2379"
			etcd = &b

			err := util.CheckHvVgAll(*etcd, *node)
			Expect(err).To(BeNil(), "Expected no error")

		})

		It("管理下のハイパーバイザーがリストされること", func() {
			statusCode, body, url, err := ep.ListHypervisors(nil)
			GinkgoWriter.Printf("Status Code: %d, Body: %s, URL: %v, Error: %v\n", statusCode, body, url, err)
			Expect(err).To(BeNil(), "Expected no error")
			Expect(statusCode).To(Equal(200), "Expected status code")
			var hypervisors api.Hypervisors
			err = json.Unmarshal(body, &hypervisors)
			Expect(err).To(BeNil(), "Expected no error unmarshalling hypervisors")
			Expect(len(hypervisors)).To(BeNumerically(">", 0), "Expected at least one hypervisor")
			for _, hv := range hypervisors {
				GinkgoWriter.Printf("Hypervisor: %+v\n", hv.NodeName)
				GinkgoWriter.Printf("    cpu: %+v¥n", hv.Cpu)
				if hv.IpAddr != nil {
					GinkgoWriter.Printf("    IP:  %+v¥n", *hv.IpAddr)
				}
				if hv.Memory != nil {
					GinkgoWriter.Printf("    Mem: %+v¥n", *hv.Memory)
				}
				GinkgoWriter.Println()
			}
		})
	})
})
