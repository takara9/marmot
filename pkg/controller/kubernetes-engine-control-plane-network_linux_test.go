package controller

import (
	"errors"
	"net"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"github.com/vishvananda/netns"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineControlPlaneNetwork", func() {
	It("derives namespace and veth names from the cluster name", func() {
		namespace, hostVeth, peerVeth, err := KubernetesEngineControlPlaneNetworkNames("demo")
		Expect(err).NotTo(HaveOccurred())
		Expect(namespace).To(Equal("mke-demo"))
		Expect(len(hostVeth)).To(BeNumerically("<=", 15))
		Expect(len(peerVeth)).To(BeNumerically("<=", 15))
		Expect(hostVeth).NotTo(Equal(peerVeth))
	})

	It("creates the control-plane namespace via the ip subprocess", func() {
		originalRunIP := controlPlaneRunIP
		DeferCleanup(func() { controlPlaneRunIP = originalRunIP })

		var got []string
		controlPlaneRunIP = func(args ...string) ([]byte, error) {
			got = append([]string(nil), args...)
			return nil, nil
		}

		Expect(createControlPlaneNamespace("mke-demo")).To(Succeed())
		Expect(got).To(Equal([]string{"netns", "add", "mke-demo"}))
	})

	It("sets up and tears down the control-plane network in order", func() {
		var calls []string
		installFakeControlPlaneNetworkOps(&calls)
		cfg, err := NewKubernetesEngineControlPlaneNetworkConfig("demo", "br-demo", "172.16.90.100/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(SetupKubernetesEngineControlPlaneNetwork(cfg)).To(Succeed())
		Expect(calls).To(Equal([]string{"netns-add:mke-demo", "veth-add", "ovs-add", "host-up", "peer-configure"}))

		calls = nil
		Expect(TeardownKubernetesEngineControlPlaneNetwork(cfg)).To(Succeed())
		Expect(calls).To(Equal([]string{"ovs-delete", "netns-delete:mke-demo"}))
	})

	It("rolls back already-created resources when a later setup step fails", func() {
		var calls []string
		installFakeControlPlaneNetworkOps(&calls)
		controlPlaneSetHostVethUp = func(string) error {
			calls = append(calls, "host-up")
			return errors.New("link failed")
		}
		cfg, err := NewKubernetesEngineControlPlaneNetworkConfig("demo", "br-demo", "172.16.90.100/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(SetupKubernetesEngineControlPlaneNetwork(cfg)).To(HaveOccurred())
		Expect(calls).To(Equal([]string{"netns-add:mke-demo", "veth-add", "ovs-add", "host-up", "ovs-delete", "netns-delete:mke-demo"}))
	})

	It("allocates the next free API server port outside used ranges", func() {
		origProbe := kubernetesEnginePortProbe
		DeferCleanup(func() { kubernetesEnginePortProbe = origProbe })
		kubernetesEnginePortProbe = func(port int) bool { return port != 26444 }
		engines := []api.KubernetesEngine{{Status: &api.Status{ApiServerPort: util.IntPtrInt(26443)}}}
		port, err := AllocateKubernetesEngineAPIServerPort(engines, 26443, 26446)
		Expect(err).NotTo(HaveOccurred())
		Expect(port).To(Equal(26445))
	})
})

var _ = Describe("DialInKubernetesEngineNetworkNamespace", func() {
	var (
		origGetCurrent = dialNamespaceGetCurrent
		origGetByName  = dialNamespaceGetByName
		origSet        = dialNamespaceSet
		origNetDial    = dialNamespaceNetDial
	)

	AfterEach(func() {
		dialNamespaceGetCurrent = origGetCurrent
		dialNamespaceGetByName = origGetByName
		dialNamespaceSet = origSet
		dialNamespaceNetDial = origNetDial
	})

	It("dials directly without touching namespaces when namespace is empty", func() {
		var gotNetwork, gotAddress string
		dialNamespaceNetDial = func(network, address string, timeout time.Duration) (net.Conn, error) {
			gotNetwork, gotAddress = network, address
			return nil, nil
		}
		dialNamespaceGetCurrent = func() (netns.NsHandle, error) {
			return netns.NsHandle(0), errors.New("must not be called")
		}

		_, err := DialInKubernetesEngineNetworkNamespace("", "tcp", "172.16.1.10:22", time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotNetwork).To(Equal("tcp"))
		Expect(gotAddress).To(Equal("172.16.1.10:22"))
	})

	It("switches into the target namespace, dials, and restores the original namespace", func() {
		origHandle := netns.NsHandle(-2)
		targetHandle := netns.NsHandle(-3)
		var setCalls []netns.NsHandle

		dialNamespaceGetCurrent = func() (netns.NsHandle, error) { return origHandle, nil }
		dialNamespaceGetByName = func(name string) (netns.NsHandle, error) {
			Expect(name).To(Equal("mke-demo"))
			return targetHandle, nil
		}
		dialNamespaceSet = func(ns netns.NsHandle) error {
			setCalls = append(setCalls, ns)
			return nil
		}
		dialNamespaceNetDial = func(network, address string, timeout time.Duration) (net.Conn, error) {
			// namespaceの切り替え後にダイヤルされることを保証する。
			Expect(setCalls).To(Equal([]netns.NsHandle{targetHandle}))
			return nil, nil
		}

		_, err := DialInKubernetesEngineNetworkNamespace("mke-demo", "tcp", "172.16.1.10:22", time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(setCalls).To(Equal([]netns.NsHandle{targetHandle, origHandle}))
	})

	It("returns an error when the target namespace cannot be opened", func() {
		dialNamespaceGetCurrent = func() (netns.NsHandle, error) { return netns.NsHandle(-2), nil }
		dialNamespaceGetByName = func(name string) (netns.NsHandle, error) {
			return netns.NsHandle(0), errors.New("no such namespace")
		}

		_, err := DialInKubernetesEngineNetworkNamespace("missing", "tcp", "172.16.1.10:22", time.Second)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to open network namespace missing"))
	})

	It("propagates the dial error and still restores the original namespace", func() {
		origHandle := netns.NsHandle(-2)
		targetHandle := netns.NsHandle(-3)
		var setCalls []netns.NsHandle

		dialNamespaceGetCurrent = func() (netns.NsHandle, error) { return origHandle, nil }
		dialNamespaceGetByName = func(name string) (netns.NsHandle, error) { return targetHandle, nil }
		dialNamespaceSet = func(ns netns.NsHandle) error {
			setCalls = append(setCalls, ns)
			return nil
		}
		dialNamespaceNetDial = func(network, address string, timeout time.Duration) (net.Conn, error) {
			return nil, errors.New("connect: no route to host")
		}

		_, err := DialInKubernetesEngineNetworkNamespace("mke-demo", "tcp", "172.16.1.10:22", time.Second)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no route to host"))
		Expect(setCalls).To(Equal([]netns.NsHandle{targetHandle, origHandle}))
	})
})

func installFakeControlPlaneNetworkOps(calls *[]string) {
	origNamespaceExists := controlPlaneNamespaceExists
	origCreateNamespace := controlPlaneCreateNamespace
	origDeleteNamespace := controlPlaneDeleteNamespace
	origCreateVeth := controlPlaneCreateVeth
	origAddOVSPort := controlPlaneAddOVSPort
	origDeleteOVSPort := controlPlaneDeleteOVSPort
	origSetHostVethUp := controlPlaneSetHostVethUp
	origConfigurePeer := controlPlaneConfigurePeer

	controlPlaneNamespaceExists = func(string) bool { return false }
	controlPlaneCreateNamespace = func(name string) error {
		*calls = append(*calls, "netns-add:"+name)
		return nil
	}
	controlPlaneDeleteNamespace = func(name string) error {
		*calls = append(*calls, "netns-delete:"+name)
		return nil
	}
	controlPlaneCreateVeth = func(string, string) error {
		*calls = append(*calls, "veth-add")
		return nil
	}
	controlPlaneAddOVSPort = func(string, string) error {
		*calls = append(*calls, "ovs-add")
		return nil
	}
	controlPlaneDeleteOVSPort = func(string, string) error {
		*calls = append(*calls, "ovs-delete")
		return nil
	}
	controlPlaneSetHostVethUp = func(string) error {
		*calls = append(*calls, "host-up")
		return nil
	}
	controlPlaneConfigurePeer = func(string, string, string, string) error {
		*calls = append(*calls, "peer-configure")
		return nil
	}

	DeferCleanup(func() {
		controlPlaneNamespaceExists = origNamespaceExists
		controlPlaneCreateNamespace = origCreateNamespace
		controlPlaneDeleteNamespace = origDeleteNamespace
		controlPlaneCreateVeth = origCreateVeth
		controlPlaneAddOVSPort = origAddOVSPort
		controlPlaneDeleteOVSPort = origDeleteOVSPort
		controlPlaneSetHostVethUp = origSetHostVethUp
		controlPlaneConfigurePeer = origConfigurePeer
	})
}
