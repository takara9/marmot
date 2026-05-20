package marmotd_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("VirtualNetworkSecurityPolicyDatapath", Ordered, func() {
	const (
		bridgeName = "br-marmot-sg-it"
		nsClient   = "marmot-sg-c"
		nsServer   = "marmot-sg-s"
		clientIf   = "veth-sgc"
		clientBrIf = "veth-sgcb"
		serverIf   = "veth-sgs"
		serverBrIf = "veth-sgsb"
		clientIP   = "10.203.0.2/24"
		serverIP   = "10.203.0.3/24"
		serverAddr = "10.203.0.3"
		tcpPort    = 18080
	)

	var (
		fabric    networkfabric.NetworkFabric
		serverCmd *exec.Cmd
	)

	requireCmd := func(name string) {
		if _, err := exec.LookPath(name); err != nil {
			Skip(fmt.Sprintf("%s is required for integration test", name))
		}
	}

	runCmd := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s %s failed: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}

	cleanupCmd := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		_, _ = cmd.CombinedOutput()
	}

	BeforeAll(func() {
		if os.Geteuid() != 0 {
			Skip("root privileges are required for netns/ovs integration test")
		}

		requireCmd("ovs-vsctl")
		requireCmd("ip")
		requireCmd("python3")
		requireCmd("curl")

		fabric = networkfabric.NewOVSFabric()

		// Ensure clean slate in case a previous run was interrupted.
		cleanupCmd("ip", "netns", "del", nsClient)
		cleanupCmd("ip", "netns", "del", nsServer)
		cleanupCmd("ovs-vsctl", "--if-exists", "del-br", bridgeName)

		runCmd("ovs-vsctl", "--may-exist", "add-br", bridgeName)
		runCmd("ip", "link", "add", clientIf, "type", "veth", "peer", "name", clientBrIf)
		runCmd("ip", "link", "add", serverIf, "type", "veth", "peer", "name", serverBrIf)

		runCmd("ip", "netns", "add", nsClient)
		runCmd("ip", "netns", "add", nsServer)
		runCmd("ip", "link", "set", clientIf, "netns", nsClient)
		runCmd("ip", "link", "set", serverIf, "netns", nsServer)

		runCmd("ovs-vsctl", "--may-exist", "add-port", bridgeName, clientBrIf)
		runCmd("ovs-vsctl", "--may-exist", "add-port", bridgeName, serverBrIf)

		runCmd("ip", "link", "set", clientBrIf, "up")
		runCmd("ip", "link", "set", serverBrIf, "up")

		runCmd("ip", "netns", "exec", nsClient, "ip", "link", "set", "lo", "up")
		runCmd("ip", "netns", "exec", nsClient, "ip", "addr", "add", clientIP, "dev", clientIf)
		runCmd("ip", "netns", "exec", nsClient, "ip", "link", "set", clientIf, "up")

		runCmd("ip", "netns", "exec", nsServer, "ip", "link", "set", "lo", "up")
		runCmd("ip", "netns", "exec", nsServer, "ip", "addr", "add", serverIP, "dev", serverIf)
		runCmd("ip", "netns", "exec", nsServer, "ip", "link", "set", serverIf, "up")

		serverCmd = exec.Command("ip", "netns", "exec", nsServer, "python3", "-m", "http.server", fmt.Sprintf("%d", tcpPort), "--bind", serverAddr)
		serverCmd.Stdout = GinkgoWriter
		serverCmd.Stderr = GinkgoWriter
		Expect(serverCmd.Start()).To(Succeed())

		Eventually(func() error {
			cmd := exec.Command("ip", "netns", "exec", nsClient, "curl", "-sS", "--max-time", "2", fmt.Sprintf("http://%s:%d", serverAddr, tcpPort))
			_, err := cmd.CombinedOutput()
			return err
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
	})

	AfterAll(func() {
		if serverCmd != nil && serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			_, _ = serverCmd.Process.Wait()
		}
		cleanupCmd("ip", "netns", "del", nsClient)
		cleanupCmd("ip", "netns", "del", nsServer)
		cleanupCmd("ovs-vsctl", "--if-exists", "del-br", bridgeName)
	})

	It("allows traffic on permitted port and blocks traffic when not permitted", func() {
		vnet := api.VirtualNetwork{
			Spec: api.VirtualNetworkSpec{
				BridgeName: util.StringPtr(bridgeName),
				SecurityPolicy: &api.VirtualNetworkSecurityPolicy{
					DefaultAction: api.Deny,
					Rules: []api.VirtualNetworkSecurityRule{
						{
							Direction:    api.Ingress,
							Protocol:     api.Tcp,
							RemoteCidr:   "10.203.0.0/24",
							PortRangeMin: tcpPort,
							PortRangeMax: tcpPort,
						},
					},
				},
			},
		}
		Expect(fabric.ApplySecurityPolicy(&vnet)).To(Succeed())

		By("permitted port should pass")
		Eventually(func() error {
			cmd := exec.Command("ip", "netns", "exec", nsClient, "curl", "-sS", "--max-time", "2", fmt.Sprintf("http://%s:%d", serverAddr, tcpPort))
			_, err := cmd.CombinedOutput()
			return err
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("unpermitted port should be blocked")
		vnet.Spec.SecurityPolicy.Rules = []api.VirtualNetworkSecurityRule{
			{
				Direction:    api.Ingress,
				Protocol:     api.Tcp,
				RemoteCidr:   "10.203.0.0/24",
				PortRangeMin: 22,
				PortRangeMax: 22,
			},
		}
		Expect(fabric.ApplySecurityPolicy(&vnet)).To(Succeed())

		Consistently(func() error {
			cmd := exec.Command("ip", "netns", "exec", nsClient, "curl", "-sS", "--max-time", "2", fmt.Sprintf("http://%s:%d", serverAddr, tcpPort))
			_, err := cmd.CombinedOutput()
			return err
		}, 6*time.Second, 1*time.Second).Should(HaveOccurred())
	})

	It("allows unmatched new traffic when defaultAction is allow", func() {
		vnet := api.VirtualNetwork{
			Spec: api.VirtualNetworkSpec{
				BridgeName: util.StringPtr(bridgeName),
				SecurityPolicy: &api.VirtualNetworkSecurityPolicy{
					DefaultAction: api.Allow,
					Rules: []api.VirtualNetworkSecurityRule{
						{
							Direction:    api.Ingress,
							Protocol:     api.Tcp,
							RemoteCidr:   "10.203.0.0/24",
							PortRangeMin: 22,
							PortRangeMax: 22,
						},
					},
				},
			},
		}
		Expect(fabric.ApplySecurityPolicy(&vnet)).To(Succeed())

		By("unmatched port should pass because defaultAction=allow")
		Eventually(func() error {
			cmd := exec.Command("ip", "netns", "exec", nsClient, "curl", "-sS", "--max-time", "2", fmt.Sprintf("http://%s:%d", serverAddr, tcpPort))
			_, err := cmd.CombinedOutput()
			return err
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
	})
})
