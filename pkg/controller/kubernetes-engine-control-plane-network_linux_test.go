package controller

import (
	"errors"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"

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
