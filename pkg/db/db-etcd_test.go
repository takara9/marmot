package db_test

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Etcd", Ordered, func() {
	var url string
	var err error
	var d *db.Database
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		url = "http://127.0.0.1:4379"
		cmd := exec.Command("docker", "run", "-d", "--name", "etcddb", "-p", "4379:2379", "-p", "4380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)

		time.Sleep(10 * time.Second) // コンテナが起動するまで待機
	}, NodeTimeout(20*time.Second))

	AfterAll(func(ctx SpecContext) {
		fmt.Println("STOPPING CONTAINER:", containerID)
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

	Describe("Test etcd", func() {
		Context("Test Connection to etcd", func() {
			It("Connection etcd", func() {
				d, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Test Version", func() {
			It("Set version", func() {
				sv := "3.2.1"
				v := api.Version{
					ClientVersion: "1.2.3",
					ServerVersion: &sv,
				}
				err := d.SetVersion(v)
				Expect(err).NotTo(HaveOccurred())
			})
			It("Get version", func() {
				v, err := d.GetVersion()
				Expect(err).NotTo(HaveOccurred())
				Expect(v.ClientVersion).To(Equal("1.2.3"))
				Expect(*v.ServerVersion).To(Equal("3.2.1"))
			})

			It("Set version with nil", func() {
				v := api.Version{
					ClientVersion: "1.2.3",
				}
				err := d.SetVersion(v)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Get version with nil", func() {
				v, err := d.GetVersion()
				Expect(err).NotTo(HaveOccurred())
				Expect(v.ClientVersion).To(Equal("1.2.3"))
				GinkgoWriter.Println("v.ServerVersion=", v.ServerVersion)
				Expect(v.ServerVersion).To(BeNil())
			})

			It("Delete version key", func() {
				err := d.DeleteJSON("version")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
