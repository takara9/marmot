package marmotd_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("Cloud-InitISO作成テスト", Ordered, func() {
	password := "12345"
	sshKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDC7ciYRXg20phLiWN4Dq4JNs5pWsMU/8sHZKesREjf9OPAyE8fegP2XkIy7ZFAV1oM+TeDQvVVrIuziJWcuoXf9/tnLLOt82zKJ89EcSUBuqERuPUrp2hqD52ff/yOFGcLGMSjtxjTZLQy40ZBUgBM8cbexqQY92mo0A9MKMbHNve0Y5FhBb2nq8EEml8qbE98hvfxScmuLCAD8OUfdgQeLqIHCCjy2IcxtazChPLyEBbcLnRGMZUFnNO8lEt8RWAw5HnZ/fI70335REQ2zctiSBWatBDOYE8anvAlek5m18BCyahxfeTxe27nz+1qslqsNtjCaJs1kWl+8u8QT8/n"
	var isoFile string

	BeforeAll(func(ctx SpecContext) {
	})

	AfterAll(func(ctx SpecContext) {
		os.Remove(isoFile) // テスト後にISOファイルを削除
	})

	Context("ISOの作成と確認", func() {
		var err error
		path := "/var/lib/marmot/isos/test-server"

		It("モックサーバー用etcdの起動", func() {
			isoFile, err = marmotd.GenerateCloudInitISO(path, password, sshKey, "")
			Expect(err).NotTo(HaveOccurred())
			fmt.Println("ISO FILE=", isoFile)
		})

		It("生成したISOファイルの内容確認", func() {
			fmt.Println("ISO FILE=", isoFile)
			fstats, err := os.Stat(isoFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(fstats.Size()).To(BeNumerically(">", 0))
			GinkgoWriter.Printf("ISOファイルが正常に生成され、内容が確認されました。")
		})

		It("生成したISOファイルの削除", func() {
			err := os.Remove(isoFile)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("ISOファイルが正常に削除されました。")
			_, err = os.Stat(isoFile)
			Expect(os.IsNotExist(err)).To(BeTrue())
			GinkgoWriter.Printf("ISOファイルの存在が確認されませんでした。")
		})

	})
})
