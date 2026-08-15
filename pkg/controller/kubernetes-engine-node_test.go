package controller

import (
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
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

	It("merges spec.nodeSpec.auth users/key with MKE's own managed root key", func() {
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Spec: api.KubernetesEngineSpec{
				Nodes: 1,
				NodeSpec: &api.KubernetesEngineNodeSpec{
					Auth: &api.Auth{
						Users:     &[]string{"alice", "root"},
						PublicKey: util.StringPtr("ssh-rsa USERKEY"),
					},
				},
			},
		}
		api.SetKubernetesEngineID(&ke, "ke123")

		server, err := buildKubernetesEngineNodeServerSpec(ke, 0, "ssh-rsa MKEKEY")
		Expect(err).NotTo(HaveOccurred())
		Expect(server.Spec.Auth).NotTo(BeNil())
		Expect(server.Spec.Auth.PublicKey).To(HaveValue(Equal("ssh-rsa MKEKEY\nssh-rsa USERKEY")))
		Expect(server.Spec.Auth.Users).To(HaveValue(Equal([]string{"root", "alice"})))
	})

	It("rejects spec.nodeSpec.auth with both user and users set", func() {
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Spec: api.KubernetesEngineSpec{
				Nodes: 1,
				NodeSpec: &api.KubernetesEngineNodeSpec{
					Auth: &api.Auth{
						User:  util.StringPtr("alice"),
						Users: &[]string{"bob"},
					},
				},
			},
		}
		api.SetKubernetesEngineID(&ke, "ke123")

		_, err := buildKubernetesEngineNodeServerSpec(ke, 0, "ssh-rsa MKEKEY")
		Expect(err).To(HaveOccurred())
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

	It("rejects an unsupported nodeSpec.network.kind", func() {
		kind := "flannel"
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Spec: api.KubernetesEngineSpec{
				Nodes:    1,
				NodeSpec: &api.KubernetesEngineNodeSpec{Network: &api.KubernetesEngineNodeNetwork{Kind: &kind}},
			},
		}
		_, err := buildKubernetesEngineNodeServerSpec(ke, 0, "ssh-rsa AAAA")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nodeSpec.network.kind"))
	})

	It("defaults nodeSpec.network.kind to bridge (none) when unset", func() {
		ke := api.KubernetesEngine{Metadata: api.Metadata{Name: "demo"}}
		Expect(kubernetesEngineNodeNetworkKind(ke)).To(Equal(kubernetesEngineNetworkKindBridge))
	})

	It("recognizes cilium as a valid nodeSpec.network.kind", func() {
		kind := "Cilium"
		ke := api.KubernetesEngine{
			Metadata: api.Metadata{Name: "demo"},
			Spec: api.KubernetesEngineSpec{
				NodeSpec: &api.KubernetesEngineNodeSpec{Network: &api.KubernetesEngineNodeNetwork{Kind: &kind}},
			},
		}
		Expect(kubernetesEngineNodeNetworkKind(ke)).To(Equal(kubernetesEngineNetworkKindCilium))
	})

	It("derives the pod CIDR from the node index", func() {
		Expect(kubernetesEnginePodCIDR(0)).To(Equal("10.244.1.0/24"))
		Expect(kubernetesEnginePodCIDR(4)).To(Equal("10.244.5.0/24"))
	})

	It("points the kubelet at systemd-resolved's real upstream resolv.conf to avoid a self-loop", func() {
		Expect(renderKubernetesEngineKubeletConfig()).To(ContainSubstring("resolvConf: /run/systemd/resolve/resolv.conf"))
	})

	It("derives the node index from its name", func() {
		index, err := kubernetesEngineNodeIndex("mke-demo-node-3")
		Expect(err).NotTo(HaveOccurred())
		Expect(index).To(Equal(2))

		_, err = kubernetesEngineNodeIndex("not-a-node-name")
		Expect(err).To(HaveOccurred())
	})

	It("reconciles cross-node pod routes for every running node when bridge CNI is used", func() {
		origRouting := runKubernetesEngineNodeRouting
		DeferCleanup(func() { runKubernetesEngineNodeRouting = origRouting })

		type call struct {
			address string
			nodeID  string
			routes  []kubernetesEngineNodeRoute
		}
		var calls []call
		runKubernetesEngineNodeRouting = func(address, privateKeyPath, namespace, nodeID string, routes []kubernetesEngineNodeRoute) error {
			calls = append(calls, call{address: address, nodeID: nodeID, routes: routes})
			return nil
		}

		ke := api.KubernetesEngine{Metadata: api.Metadata{Name: "demo"}}
		api.SetKubernetesEngineID(&ke, "ke123")
		servers := []api.Server{
			{
				Metadata: api.Metadata{Name: "mke-demo-node-1"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{
					{Networkname: "mke-demo", Address: util.StringPtr("172.16.1.10")},
				}},
				Status: &api.Status{StatusCode: db.SERVER_RUNNING},
			},
			{
				Metadata: api.Metadata{Name: "mke-demo-node-2"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{
					{Networkname: "mke-demo", Address: util.StringPtr("172.16.1.11")},
				}},
				Status: &api.Status{StatusCode: db.SERVER_RUNNING},
			},
			{
				Metadata: api.Metadata{Name: "mke-demo-node-3"},
				Spec: api.ServerSpec{NetworkInterface: &[]api.NetworkInterface{
					{Networkname: "mke-demo", Address: util.StringPtr("172.16.1.12")},
				}},
				Status: &api.Status{StatusCode: db.SERVER_PENDING},
			},
		}

		Expect(reconcileKubernetesEngineNodeRoutes(ke, servers)).To(Succeed())
		Expect(calls).To(HaveLen(2))
		for _, c := range calls {
			Expect(c.routes).To(HaveLen(1))
		}
	})
})
