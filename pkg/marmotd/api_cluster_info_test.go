package marmotd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

var _ = Describe("Cluster info API handler", Ordered, func() {
	const (
		port      = "26380"
		etcdImage = "ghcr.io/takara9/etcd:3.6.5"
	)

	var (
		containerID string
		d           *db.Database
		s           *marmotd.Server
	)

	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/marmot/cluster/info", nil)
		rec := httptest.NewRecorder()
		Expect(s.ApiGetMarmotClusterInfo(echo.New().NewContext(req, rec))).To(Succeed())
		return rec
	}

	BeforeAll(func(ctx0 SpecContext) {
		output, err := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%s:2379", port), etcdImage).CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("failed to start etcd container: %s (%v)", string(output), err))
		}
		containerID = strings.TrimSpace(string(output))
		time.Sleep(10 * time.Second)

		url := fmt.Sprintf("http://127.0.0.1:%s", port)
		d, err = db.NewDatabase(url)
		Expect(err).NotTo(HaveOccurred())
		s = &marmotd.Server{Ma: &marmotd.Marmot{NodeName: "hvc", EtcdUrl: url, Db: d}}
	})

	AfterAll(func(ctx0 SpecContext) {
		if d != nil {
			_ = d.Close()
		}
		if containerID != "" {
			_, _ = exec.Command("docker", "stop", containerID).CombinedOutput()
		}
	})

	It("returns 404 when the cluster is not enabled", func() {
		rec := call()
		Expect(rec.Code).To(Equal(http.StatusNotFound), rec.Body.String())
	})

	It("returns the stored cluster info", func() {
		Expect(d.PutClusterInfo(api.ClusterInfo{ClusterId: "6f1c0c0e-0000-4000-8000-000000000000", MasterNode: "hv3"})).To(Succeed())

		rec := call()
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())
		var got api.ClusterInfo
		Expect(json.Unmarshal(rec.Body.Bytes(), &got)).To(Succeed())
		Expect(got.ClusterId).To(Equal("6f1c0c0e-0000-4000-8000-000000000000"))
		Expect(got.MasterNode).To(Equal("hv3"))
	})
})
