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

	Describe("findSSHConnectTargetIndex", func() {
		It("finds the first positional host after ssh options", func() {
			idx, err := findSSHConnectTargetIndex([]string{"-i", "~/.ssh/id_ed25519", "-o", "StrictHostKeyChecking=no", "server-20", "hostname"})
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(4))
		})

		It("accepts repeated tty shorthand option -tt", func() {
			idx, err := findSSHConnectTargetIndex([]string{"-tt", "server-20", "hostname"})
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(1))
		})

		It("finds host after -l user option", func() {
			idx, err := findSSHConnectTargetIndex([]string{"-l", "ubuntu", "server-20", "hostname"})
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(2))
		})
	})

	Describe("rewriteSSHArgsForMarmot", func() {
		It("rewrites direct server target to host-bridge IP", func() {
			servers := []api.Server{{
				Metadata: api.Metadata{Name: "server-20"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
					Address:     util.StringPtr("192.168.10.50"),
				}}},
			}}

			args, err := rewriteSSHArgsForMarmot(servers, []string{"server-20", "hostname"})
			Expect(err).NotTo(HaveOccurred())
			Expect(args).To(Equal([]string{"192.168.10.50", "hostname"}))
		})

		It("rewrites ssh-style args while preserving options", func() {
			servers := []api.Server{{
				Metadata: api.Metadata{Name: "server-20"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
					Address:     util.StringPtr("192.168.10.50"),
				}}},
			}}

			args, err := rewriteSSHArgsForMarmot(servers, []string{"-i", "~/.ssh/id_ed25519", "-o", "StrictHostKeyChecking=no", "server-20", "hostname"})
			Expect(err).NotTo(HaveOccurred())
			Expect(args).To(Equal([]string{"-i", "~/.ssh/id_ed25519", "-o", "StrictHostKeyChecking=no", "192.168.10.50", "hostname"}))
		})

		It("keeps -tt and rewrites only destination", func() {
			servers := []api.Server{{
				Metadata: api.Metadata{Name: "server-20"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
					Address:     util.StringPtr("192.168.10.50"),
				}}},
			}}

			args, err := rewriteSSHArgsForMarmot(servers, []string{"-tt", "server-20", "hostname"})
			Expect(err).NotTo(HaveOccurred())
			Expect(args).To(Equal([]string{"-tt", "192.168.10.50", "hostname"}))
		})

		It("moves ssh options before the destination when -- is used", func() {
			servers := []api.Server{{
				Metadata: api.Metadata{Name: "server-20"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{{
					Networkname: "host-bridge",
					Address:     util.StringPtr("192.168.10.50"),
				}}},
			}}

			args, err := rewriteSSHArgsForMarmot(servers, []string{"server-20", "--", "-i", "vmkey"})
			Expect(err).NotTo(HaveOccurred())
			Expect(args).To(Equal([]string{"-i", "vmkey", "192.168.10.50"}))
		})
	})

	Describe("ssh command configuration", func() {
		It("disables cobra flag parsing for ssh passthrough", func() {
			Expect(sshCmd.DisableFlagParsing).To(BeTrue())
		})
	})
})
