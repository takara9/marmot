package marmotd

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCloudInitISOCleanup(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	suiteConfig.LabelFilter = "cloud-init-cleanup"
	RunSpecs(t, "Cloud-init ISO Cleanup Suite", suiteConfig, reporterConfig)
}

var _ = Describe("cloud-init ISO の削除", Label("cloud-init-cleanup"), func() {
	var baseDir string

	BeforeEach(func() {
		baseDir = GinkgoT().TempDir()
	})

	createServerDir := func(id string, files ...string) string {
		dir := filepath.Join(baseDir, id)
		Expect(os.MkdirAll(dir, 0755)).To(Succeed())
		for _, name := range files {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644)).To(Succeed())
		}
		return dir
	}

	It("ISO と空になったディレクトリを削除する", func() {
		dir := createServerDir("srv-1", "cloud-init.iso")

		Expect(removeServerCloudInitISO(baseDir, "srv-1")).To(Succeed())
		Expect(dir).NotTo(BeADirectory())
	})

	It("対象が存在しなくても成功する", func() {
		Expect(removeServerCloudInitISO(baseDir, "missing")).To(Succeed())
	})

	It("他のファイルが残るディレクトリは削除せずエラーを返し、解消後の再試行で成功する", func() {
		dir := createServerDir("srv-2", "cloud-init.iso", "other")

		Expect(removeServerCloudInitISO(baseDir, "srv-2")).NotTo(Succeed())
		Expect(filepath.Join(dir, "other")).To(BeARegularFile())
		Expect(filepath.Join(dir, "cloud-init.iso")).NotTo(BeAnExistingFile())

		Expect(os.Remove(filepath.Join(dir, "other"))).To(Succeed())
		Expect(removeServerCloudInitISO(baseDir, "srv-2")).To(Succeed())
		Expect(dir).NotTo(BeADirectory())
	})

	DescribeTable("不正なサーバー ID を拒否する",
		func(id string) {
			Expect(removeServerCloudInitISO(baseDir, id)).NotTo(Succeed())
		},
		Entry("空文字", ""),
		Entry("空白のみ", " "),
		Entry("カレントディレクトリ", "."),
		Entry("親ディレクトリ", ".."),
		Entry("親ディレクトリを含むパス", "../x"),
		Entry("区切り文字を含む", "a/b"),
	)
})
