package db_test

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("ClusterInfo", Ordered, func() {
	port := "12380"
	url := fmt.Sprintf("http://127.0.0.1:%s", port)
	var containerID string
	var d *db.Database

	BeforeAll(func(ctx SpecContext) {
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%s:2379", port), "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = strings.TrimSpace(string(output))
		time.Sleep(10 * time.Second)

		d, err = db.NewDatabase(url)
		Expect(err).NotTo(HaveOccurred())
	}, NodeTimeout(30*time.Second))

	AfterAll(func(ctx SpecContext) {
		_, _ = exec.Command("docker", "stop", containerID).CombinedOutput()
	}, NodeTimeout(20*time.Second))

	It("クラスタ情報が無ければ ErrNotFound を返す", func() {
		_, err := d.GetClusterInfo()
		Expect(err).To(MatchError(db.ErrNotFound))
	})

	It("保存したクラスタ情報を取得できる", func() {
		now := time.Now().UTC().Truncate(time.Second)
		Expect(d.PutClusterInfo(api.ClusterInfo{
			ClusterId:         "6f1c0c0e-0000-4000-8000-000000000000",
			MasterNode:        "hv3",
			CreationTimeStamp: &now,
		})).To(Succeed())

		got, err := d.GetClusterInfo()
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ClusterId).To(Equal("6f1c0c0e-0000-4000-8000-000000000000"))
		Expect(got.MasterNode).To(Equal("hv3"))
		Expect(got.CreationTimeStamp.Equal(now)).To(BeTrue())
	})
})
