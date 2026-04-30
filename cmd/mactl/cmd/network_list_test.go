package cmd

import (
	"strings"

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
		statusText := "ACTIVE"

		output := formatNetworkListText([]api.VirtualNetwork{{
			Id: "net01",
			Metadata: &api.Metadata{
				Name:     &name,
				NodeName: &nodeName,
			},
			Spec: &api.VirtualNetworkSpec{
				BridgeName: &bridgeName,
			},
			Status: &api.Status{
				Status:     &statusText,
				StatusCode: int(db.NETWORK_ACTIVE),
			},
		}})

		Expect(strings.Contains(output, "NETWORK-ID")).To(BeTrue(), output)
		Expect(strings.Contains(output, "NODE-NAME")).To(BeTrue(), output)
		Expect(strings.Contains(output, "node-01")).To(BeTrue(), output)
		Expect(strings.Contains(output, "test-net-1")).To(BeTrue(), output)
		Expect(strings.Contains(output, db.NetworkStatus[int(db.NETWORK_ACTIVE)])).To(BeTrue(), output)
	})
})