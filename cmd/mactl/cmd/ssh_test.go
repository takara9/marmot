package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("ssh helper functions", func() {
	Describe("parseSSHLoginTarget", func() {
		It("parses server name without user", func() {
			user, serverName, err := parseSSHLoginTarget("server-20")
			Expect(err).NotTo(HaveOccurred())
			Expect(user).To(Equal(""))
			Expect(serverName).To(Equal("server-20"))
		})

		It("parses user and server name", func() {
			user, serverName, err := parseSSHLoginTarget("ubuntu@server-20")
			Expect(err).NotTo(HaveOccurred())
			Expect(user).To(Equal("ubuntu"))
			Expect(serverName).To(Equal("server-20"))
		})

		It("returns error for malformed user at target", func() {
			_, _, err := parseSSHLoginTarget("ubuntu@")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid ssh target"))
		})
	})

	Describe("buildSSHTargetAddress", func() {
		It("returns IP when user is empty", func() {
			target := buildSSHTargetAddress("", "192.168.10.50")
			Expect(target).To(Equal("192.168.10.50"))
		})

		It("returns user at IP when user is specified", func() {
			target := buildSSHTargetAddress("ubuntu", "192.168.10.50")
			Expect(target).To(Equal("ubuntu@192.168.10.50"))
		})
	})

	Describe("findServerByName", func() {
		It("returns a matching server", func() {
			servers := []api.Server{
				{Metadata: api.Metadata{Name: "alpha"}},
				{Metadata: api.Metadata{Name: "beta"}},
			}

			server, err := findServerByName(servers, "beta")
			Expect(err).NotTo(HaveOccurred())
			Expect(server).NotTo(BeNil())
			Expect(server.Metadata.Name).To(Equal("beta"))
		})

		It("returns error when server is not found", func() {
			servers := []api.Server{{Metadata: api.Metadata{Name: "alpha"}}}

			_, err := findServerByName(servers, "gamma")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("resolveHostBridgeAddress", func() {
		It("returns host-bridge address", func() {
			nics := []api.NetworkInterface{
				{Networkname: "private-net", Address: util.StringPtr("10.0.0.2")},
				{Networkname: "host-bridge", Address: util.StringPtr("192.168.10.50")},
			}
			server := api.Server{Spec: api.ServerSpec{NetworkInterface: &nics}}

			address, err := resolveHostBridgeAddress(server)
			Expect(err).NotTo(HaveOccurred())
			Expect(address).To(Equal("192.168.10.50"))
		})

		It("returns error when host-bridge is missing", func() {
			nics := []api.NetworkInterface{{Networkname: "private-net", Address: util.StringPtr("10.0.0.2")}}
			server := api.Server{Spec: api.ServerSpec{NetworkInterface: &nics}}

			_, err := resolveHostBridgeAddress(server)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected to host-bridge"))
		})

		It("returns error when host-bridge address is empty", func() {
			nics := []api.NetworkInterface{{Networkname: "host-bridge"}}
			server := api.Server{Spec: api.ServerSpec{NetworkInterface: &nics}}

			_, err := resolveHostBridgeAddress(server)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no static address"))
		})
	})

	Describe("composeSSHArgs", func() {
		It("appends target when no extra args", func() {
			args := composeSSHArgs("192.168.10.50", nil)
			Expect(args).To(Equal([]string{"192.168.10.50"}))
		})

		It("places target after options", func() {
			args := composeSSHArgs("192.168.10.50", []string{"-i", "~/.ssh/id_ed25519", "-o", "StrictHostKeyChecking=no"})
			Expect(args).To(Equal([]string{"-i", "~/.ssh/id_ed25519", "-o", "StrictHostKeyChecking=no", "192.168.10.50"}))
		})

		It("supports remote command with -- separator", func() {
			args := composeSSHArgs("192.168.10.50", []string{"-i", "~/.ssh/id_ed25519", "--", "uname", "-a"})
			Expect(args).To(Equal([]string{"-i", "~/.ssh/id_ed25519", "192.168.10.50", "uname", "-a"}))
		})
	})
})
