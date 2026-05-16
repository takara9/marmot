package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("console endpoint resolution", func() {
	It("builds target host:port from nodeName and cluster ip", func() {
		nodeA := "marmot-a"
		nodeB := "marmot-b"
		ipA := "10.0.0.11"
		ipB := "10.0.0.12"

		statuses := []api.HostStatus{
			{NodeName: &nodeA, IpAddress: &ipA},
			{NodeName: &nodeB, IpAddress: &ipB},
		}

		hostPort, err := hostPortForNode(statuses, "marmot-b", "127.0.0.1:8750")
		Expect(err).NotTo(HaveOccurred())
		Expect(hostPort).To(Equal("10.0.0.12:8750"))
	})

	It("returns an error when target node is missing", func() {
		nodeA := "marmot-a"
		ipA := "10.0.0.11"
		statuses := []api.HostStatus{{NodeName: &nodeA, IpAddress: &ipA}}

		_, err := hostPortForNode(statuses, "marmot-z", "127.0.0.1:8750")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("was not found"))
	})

	It("returns an error when target node has no ipAddress", func() {
		nodeA := "marmot-a"
		statuses := []api.HostStatus{{NodeName: &nodeA}}

		_, err := hostPortForNode(statuses, "marmot-a", "127.0.0.1:8750")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("has no ipAddress"))
	})
})

var _ = Describe("portFromHostPort", func() {
	It("extracts port from host:port", func() {
		port, err := portFromHostPort("127.0.0.1:8750")
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal("8750"))
	})

	It("extracts port from bracketed ipv6 host:port", func() {
		port, err := portFromHostPort("[2001:db8::1]:9443")
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal("9443"))
	})

	It("fails for host without port", func() {
		_, err := portFromHostPort("localhost")
		Expect(err).To(HaveOccurred())
	})
})
