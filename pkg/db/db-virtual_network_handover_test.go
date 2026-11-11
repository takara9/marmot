package db_test

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("ネットワーク head の引き継ぎ", Ordered, func() {
	port := "16380"
	url := fmt.Sprintf("http://127.0.0.1:%s", port)
	var containerID string
	var v *db.Database

	BeforeAll(func(ctx SpecContext) {
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%s:2379", port), "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12])
		time.Sleep(10 * time.Second)

		v, err = db.NewDatabase(url)
		Expect(err).NotTo(HaveOccurred())
	}, NodeTimeout(30*time.Second))

	AfterAll(func(ctx SpecContext) {
		_, _ = exec.Command("docker", "stop", containerID).CombinedOutput()
	}, NodeTimeout(20*time.Second))

	newFollower := func(head api.VirtualNetwork, node string, statusCode int) string {
		id, err := v.MakeFollowerVirtualNetworkEntry(head, node, api.VirtualNetworkID(head))
		Expect(err).NotTo(HaveOccurred())
		f, err := v.GetVirtualNetworkById(id)
		Expect(err).NotTo(HaveOccurred())
		f.Status.StatusCode = statusCode
		Expect(v.UpdateVirtualNetworkById(id, f)).To(Succeed())
		return id
	}

	var head api.VirtualNetwork
	var followerB, followerC string

	It("head と follower を用意する", func() {
		labels := map[string]interface{}{}
		db.SetNetworkSyncLabels(labels, "head", "", "node-a")
		created, err := v.CreateVirtualNetwork(api.VirtualNetwork{
			Metadata: api.Metadata{Name: db.ManagementNetworkName, NodeName: util.StringPtr("node-a"), Labels: &labels},
			Spec:     api.VirtualNetworkSpec{IPNetworkAddress: util.StringPtr("10.245.0.0/16")},
		})
		Expect(err).NotTo(HaveOccurred())
		head = created
		followerB = newFollower(head, "node-b", db.NETWORK_WAITING)
		followerC = newFollower(head, "node-c", db.NETWORK_ACTIVE)
	})

	It("候補に follower が無ければ ErrNoHeadCandidate を返し、何も変更しない", func() {
		_, err := v.HandOverNetworkHead(api.VirtualNetworkID(head), []string{"node-x"})
		Expect(err).To(MatchError(db.ErrNoHeadCandidate))

		got, err := v.GetVirtualNetworkByName(db.ManagementNetworkName)
		Expect(err).NotTo(HaveOccurred())
		Expect(*got.Metadata.NodeName).To(Equal("node-a"))
	})

	It("ACTIVE の follower を優先して head を移し、その follower は削除する", func() {
		node, err := v.HandOverNetworkHead(api.VirtualNetworkID(head), []string{"node-b", "node-c"})
		Expect(err).NotTo(HaveOccurred())
		Expect(node).To(Equal("node-c"))

		got, err := v.GetVirtualNetworkById(api.VirtualNetworkID(head))
		Expect(err).NotTo(HaveOccurred())
		Expect(*got.Metadata.NodeName).To(Equal("node-c"))
		Expect(db.GetNetworkSyncRole(*got.Metadata.Labels)).To(Equal("head"))
		Expect(db.GetHeadNetworkNodeName(*got.Metadata.Labels)).To(Equal("node-c"))

		_, err = v.GetVirtualNetworkById(followerC)
		Expect(err).To(MatchError(db.ErrNotFound))

		b, err := v.GetVirtualNetworkById(followerB)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.GetHeadNetworkID(*b.Metadata.Labels)).To(Equal(api.VirtualNetworkID(head)))
		Expect(db.GetHeadNetworkNodeName(*b.Metadata.Labels)).To(Equal("node-c"))

		heads := 0
		nets, err := v.GetVirtualNetworks()
		Expect(err).NotTo(HaveOccurred())
		for _, n := range nets {
			if n.Metadata.Labels == nil || db.GetNetworkSyncRole(*n.Metadata.Labels) != "follower" {
				heads++
			}
		}
		Expect(heads).To(Equal(1))
	})

	It("follower への head は拒否する", func() {
		_, err := v.HandOverNetworkHead(followerB, []string{"node-b"})
		Expect(err).To(HaveOccurred())
	})
})
