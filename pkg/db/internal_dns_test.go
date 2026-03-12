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

var _ = Describe("InternalDNS", Ordered, func() {
	var port string = "10379"
	var url string = fmt.Sprintf("http://127.0.0.1:%s", port)
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		opts := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}
		logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
		slog.SetDefault(logger)

		// Dockerコンテナを起動
		cmd := exec.Command("docker", "run", "-d", "--name", "dnsEtcdDb", "-p", fmt.Sprintf("%s:2379", port), "ghcr.io/takara9/etcd:3.6.5")
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

	Describe("IPアドレス管理テスト", Ordered, func() {
		var d *db.Database
		Context("DNS名とIPv4の登録", func() {
			var testHostname = "test1"
			var subdomain = "test-net-1"
			var testIpAddress = "192.168.100.2"
			var err error
			It("データベース接続の生成", func() {
				d, err = db.NewDatabase(url)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ホスト名とIPv4アドレスの登録", func() {
				err := d.PutDnsEntry(testHostname, subdomain, testIpAddress)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ホスト名からIPv4の取得", func() {
				ipAddress, err := d.GetDnsEntry(testHostname, subdomain)
				Expect(err).NotTo(HaveOccurred())
				Expect(ipAddress).To(Equal(testIpAddress))
			})

			It("ホスト名を指定したエントリーの削除", func() {
				err := d.DeleteDnsEntryByName(testHostname, subdomain)
				Expect(err).NotTo(HaveOccurred())
			})

			It("エントリー削除の確認", func() {
				_, err := d.GetDnsEntry(testHostname, subdomain)
				Expect(err).To(HaveOccurred())
			})

		})

		Context("DNSクエリーのによるアドレス解決 IPv4", func() {
		})

		Context("登録の削除 IPv4", func() {
		})

		Context("DNS名とIPv6の登録", func() {
		})

		Context("DNSクエリーのによるアドレス解決 IPv6", func() {
		})

		Context("登録の削除 IPv6", func() {
		})
	})
})
