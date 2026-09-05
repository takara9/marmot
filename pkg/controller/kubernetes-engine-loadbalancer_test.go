package controller

import (
	"github.com/takara9/marmot/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineLoadBalancer", func() {
	It("is enabled only when nodeSpec.network.external is host-bridge", func() {
		ke := api.KubernetesEngine{Metadata: api.Metadata{Name: "demo"}}
		Expect(kubernetesEngineLoadBalancerEnabled(ke)).To(BeFalse())

		defaultExternal := "default"
		ke.Spec.NodeSpec = &api.KubernetesEngineNodeSpec{
			Network: &api.KubernetesEngineNodeNetwork{External: &defaultExternal},
		}
		Expect(kubernetesEngineLoadBalancerEnabled(ke)).To(BeFalse())

		hostBridge := "host-bridge"
		ke.Spec.NodeSpec.Network.External = &hostBridge
		Expect(kubernetesEngineLoadBalancerEnabled(ke)).To(BeTrue())
	})

	It("builds a load balancer server spec with host-bridge and cluster networks", func() {
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Spec:     api.KubernetesEngineSpec{Nodes: 1},
		}
		api.SetKubernetesEngineID(&ke, "ke123")

		server, err := buildKubernetesEngineLoadBalancerServerSpec(ke, "ssh-rsa AAAA")
		Expect(err).NotTo(HaveOccurred())
		Expect(server.Metadata.Name).To(Equal("mke-demo-lb"))
		Expect(server.Spec.Cpu).To(HaveValue(Equal(defaultKubernetesEngineLoadBalancerCPU)))
		Expect(server.Spec.Memory).To(HaveValue(Equal(defaultKubernetesEngineLoadBalancerMemory)))
		Expect(server.Spec.NetworkInterface).To(HaveValue(HaveLen(2)))
		nics := *server.Spec.NetworkInterface
		Expect(nics[0].Networkname).To(Equal("host-bridge"))
		Expect(nics[1].Networkname).To(Equal("mke-demo"))
		Expect((*server.Metadata.Labels)[kubernetesEngineNodeLabelOwner]).To(Equal("ke123"))
		Expect((*server.Metadata.Labels)[kubernetesEngineNodeLabelRole]).To(Equal(kubernetesEngineLoadBalancerRoleValue))
	})

	It("rejects an empty public key", func() {
		ke := api.KubernetesEngine{Metadata: api.Metadata{Name: "demo"}}
		_, err := buildKubernetesEngineLoadBalancerServerSpec(ke, "  ")
		Expect(err).To(HaveOccurred())
	})

	It("includes the custom marmotd CA file when HTTPS is enabled", func() {
		unit := kubernetesEngineLoadBalancerControllerUnit("https://10.0.0.10:8750", kubernetesEngineLoadBalancerMarmotdCAPath, "ke123", false)
		Expect(unit).To(ContainSubstring("--marmotd-ca-file=/etc/marmot/mke-lb-marmotd-ca.pem"))
		Expect(unit).NotTo(ContainSubstring("--cloud-controller-manager-enabled"))
	})

	It("includes the cloud-controller-manager-enabled flag when CCM is enabled for the cluster", func() {
		unit := kubernetesEngineLoadBalancerControllerUnit("https://10.0.0.10:8750", "", "ke123", true)
		Expect(unit).To(ContainSubstring("--cloud-controller-manager-enabled=true"))
	})

	It("skips provisioning entirely when the load balancer is disabled", func() {
		ke := api.KubernetesEngine{Metadata: api.Metadata{Name: "demo"}}
		ready, err := ProvisionKubernetesEngineLoadBalancer(nil, nil, ke)
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
	})
})
