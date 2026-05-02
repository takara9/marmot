package cmd

import (
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("formatNetworkListText", func() {
	It("includes the header and node name column in text output", func() {
		name := "test-net-1"
		nodeName := "node-01"
		bridgeName := "virbr100"
		ipNet := "10.10.0.0/24"
		statusText := "ACTIVE"

		output := formatNetworkListText([]api.VirtualNetwork{{
			Id: "net01",
			Metadata: &api.Metadata{
				Name:     &name,
				NodeName: &nodeName,
			},
			Spec: &api.VirtualNetworkSpec{
				BridgeName:       &bridgeName,
				IPNetworkAddress: &ipNet,
			},
			Status: &api.Status{
				Status:     &statusText,
				StatusCode: int(db.NETWORK_ACTIVE),
			},
		}})

		Expect(strings.Contains(output, "NETWORK-ID")).To(BeTrue(), output)
		Expect(strings.Contains(output, "NODE-NAME")).To(BeTrue(), output)
		Expect(strings.Contains(output, "IP-NET")).To(BeTrue(), output)
		Expect(strings.Contains(output, "node-01")).To(BeTrue(), output)
		Expect(strings.Contains(output, "test-net-1")).To(BeTrue(), output)
		Expect(strings.Contains(output, "10.10.0.0/24")).To(BeTrue(), output)
		Expect(strings.Contains(output, db.NetworkStatus[int(db.NETWORK_ACTIVE)])).To(BeTrue(), output)
	})
})

var _ = Describe("network list sort", func() {
	It("sorts by Status.creationTimeStamp in ascending order", func() {
		now := time.Now()
		earlier := now.Add(-2 * time.Minute)
		middle := now.Add(-1 * time.Minute)

		data := []api.VirtualNetwork{
			{
				Id: "net-late",
				Status: &api.Status{
					CreationTimeStamp: &now,
				},
			},
			{
				Id: "net-early",
				Status: &api.Status{
					CreationTimeStamp: &earlier,
				},
			},
			{
				Id: "net-mid",
				Status: &api.Status{
					CreationTimeStamp: &middle,
				},
			},
		}

		sort.SliceStable(data, func(i, j int) bool {
			ti := creationTime(data[i].Status)
			tj := creationTime(data[j].Status)

			hasI := !ti.IsZero()
			hasJ := !tj.IsZero()
			if hasI != hasJ {
				return hasI
			}

			if !ti.Equal(tj) {
				return ti.Before(tj)
			}

			return data[i].Id < data[j].Id
		})

		Expect(data[0].Id).To(Equal("net-early"))
		Expect(data[1].Id).To(Equal("net-mid"))
		Expect(data[2].Id).To(Equal("net-late"))
	})
})

var _ = Describe("filterHeadSyncRoleNetworks", func() {
	It("returns only networks whose Metadata.labels.syncRole is head", func() {
		headLabels := map[string]interface{}{"syncRole": "head"}
		followerLabels := map[string]interface{}{"syncRole": "follower"}

		data := []api.VirtualNetwork{
			{Id: "net-head", Metadata: &api.Metadata{Labels: &headLabels}},
			{Id: "net-follower", Metadata: &api.Metadata{Labels: &followerLabels}},
			{Id: "net-no-meta"},
			{Id: "net-no-labels", Metadata: &api.Metadata{}},
		}

		filtered := filterHeadSyncRoleNetworks(data)

		Expect(filtered).To(HaveLen(1))
		Expect(filtered[0].Id).To(Equal("net-head"))
	})

	It("keeps all records when show-all flag is used by skipping this filter", func() {
		headLabels := map[string]interface{}{"syncRole": "head"}
		followerLabels := map[string]interface{}{"syncRole": "follower"}
		data := []api.VirtualNetwork{
			{Id: "net-head", Metadata: &api.Metadata{Labels: &headLabels}},
			{Id: "net-follower", Metadata: &api.Metadata{Labels: &followerLabels}},
		}

		all := filterNetworksForList(data, true)
		headOnly := filterNetworksForList(data, false)

		Expect(all).To(HaveLen(2))
		Expect(headOnly).To(HaveLen(1))
		Expect(headOnly[0].Id).To(Equal("net-head"))
	})
})
