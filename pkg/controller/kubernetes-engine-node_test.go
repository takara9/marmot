package controller

import (
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineNode", func() {
	It("builds a node server spec with resources, networks and labels", func() {
		external := "host-bridge"
		cpu := 4
		memory := 8192
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Spec: api.KubernetesEngineSpec{
				Nodes: 2,
				NodeSpec: &api.KubernetesEngineNodeSpec{
					Cpu:     &cpu,
					Memory:  &memory,
					Network: &api.KubernetesEngineNodeNetwork{External: &external},
				},
			},
		}
		api.SetKubernetesEngineID(&ke, "ke123")

		server, err := buildKubernetesEngineNodeServerSpec(ke, 1, "ssh-rsa AAAA")
		Expect(err).NotTo(HaveOccurred())
		Expect(server.Metadata.Name).To(Equal("mke-demo-node-2"))
		Expect(server.Spec.Cpu).To(HaveValue(Equal(cpu)))
		Expect(server.Spec.Memory).To(HaveValue(Equal(memory)))
		Expect(server.Spec.NetworkInterface).To(HaveValue(HaveLen(2)))
		nics := *server.Spec.NetworkInterface
		Expect(nics[0].Networkname).To(Equal("host-bridge"))
		Expect(nics[1].Networkname).To(Equal("mke-demo"))
		Expect(server.Metadata.Labels).NotTo(BeNil())
		Expect((*server.Metadata.Labels)[kubernetesEngineNodeLabelOwner]).To(Equal("ke123"))
		Expect(server.Spec.Auth).NotTo(BeNil())
		Expect(server.Spec.Auth.PublicKey).To(HaveValue(Equal("ssh-rsa AAAA")))
	})

	It("reports nodes ready only when every listed node has a True Ready condition", func() {
		data := []byte(`{"items":[
			{"metadata":{"name":"node-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
			{"metadata":{"name":"node-2"},"status":{"conditions":[{"type":"Ready","status":"False"}]}}
		]}`)
		ready, err := kubernetesEngineNodesReady(data, []string{"node-1", "node-2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())

		readyData := []byte(`{"items":[
			{"metadata":{"name":"node-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
			{"metadata":{"name":"node-2"},"status":{"conditions":[{"type":"Ready","status":"True"}]}}
		]}`)
		ready, err = kubernetesEngineNodesReady(readyData, []string{"node-1", "node-2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
	})

	It("resolves a node's internal IP from its network interfaces", func() {
		server := api.Server{Metadata: api.Metadata{Name: "node-1"}, Spec: api.ServerSpec{
			NetworkInterface: &[]api.NetworkInterface{
				{Networkname: "default"},
				{Networkname: "mke-demo", Address: util.StringPtr("172.16.1.10")},
			},
		}}
		address, err := kubernetesEngineNodeInternalIP(server, "mke-demo")
		Expect(err).NotTo(HaveOccurred())
		Expect(address).To(Equal("172.16.1.10"))
	})
})
