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

var _ = Describe("Servers", Ordered, func() {
	var url string = "http://127.0.0.1:7379"
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "jobEtcdDb", "-p", "7379:2379", "-p", "7380:2380", "ghcr.io/takara9/etcd:3.6.5")
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

	Describe("サーバー管理テスト", func() {
		var v *db.Database
		var server *api.Server
		var serverSpec api.Server

		Context("基本アクセス", func() {
			var err error
			It("データベース接続の生成", func() {
				v, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("サーバーの作成 #1", func() {
				server = &api.Server{
					Metadata: &api.Metadata{
						Name: util.StringPtr("server01"),
					},
					Spec: &api.ServerSpec{
						Cpu:    util.IntPtrInt(2),
						Memory: util.IntPtrInt(4096),
					},
				}
				serverSpec, err = v.CreateServer(*server)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created server with ID:", serverSpec.Id)
			})

			It("Keyからサーバー情報を取得", func() {
				srv, err := v.GetServerById(serverSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				bytes, err := json.MarshalIndent(srv, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(bytes))
			})

			It("サーバーの状態更新 #1", func() {
				srv := api.Server{
					Status: &api.Status{
						Status: util.IntPtrInt(db.VOLUME_AVAILABLE),
					},
				}
				err = v.UpdateServer(serverSpec.Id, srv)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Keyからサーバー情報を取得", func() {
				srv, err := v.GetServerById(serverSpec.Id)
				Expect(err).NotTo(HaveOccurred())
				jsonData, err := json.MarshalIndent(srv, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(string(jsonData))
			})

			It("サーバーの作成 #2", func() {
				server = &api.Server{
					Metadata: &api.Metadata{
						Name: util.StringPtr("server02"),
					},
					Spec: &api.ServerSpec{
						Cpu:    util.IntPtrInt(2),
						Memory: util.IntPtrInt(4096),
					},
				}
				serverSpec, err = v.CreateServer(*server)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created server with ID:", serverSpec.Id)
			})

			It("サーバーの作成 #3", func() {
				server = &api.Server{
					Metadata: &api.Metadata{
						Name: util.StringPtr("server03"),
					},
					Spec: &api.ServerSpec{
						Cpu:    util.IntPtrInt(2),
						Memory: util.IntPtrInt(4096),
					},
				}
				serverSpec, err = v.CreateServer(*server)
				Expect(err).NotTo(HaveOccurred())
				fmt.Println("Created server with ID:", serverSpec.Id)
			})

			It("サーバーの一覧取得", func() {
				servers, err := v.GetServers()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(servers)).To(Equal(3))
				fmt.Println("サーバー一覧:")
				for _, srv := range servers {
					jsonData, err := json.MarshalIndent(srv, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})

			It("サーバーの削除", func() {
				servers, err := v.GetServers()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(servers)).To(Equal(3))
				err = v.DeleteServerById(servers[0].Id)
				Expect(err).NotTo(HaveOccurred())
			})

			It("サーバーの一覧取得", func() {
				servers, err := v.GetServers()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(servers)).To(Equal(2))
				fmt.Println("サーバー一覧:")
				for _, srv := range servers {
					jsonData, err := json.MarshalIndent(srv, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Println(string(jsonData))
				}
			})
		})
	})
})
