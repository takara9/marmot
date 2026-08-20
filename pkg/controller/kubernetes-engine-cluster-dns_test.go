package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("kubernetesEngineClusterDNSManifests", func() {
	It("returns CoreDNS manifests supported by applyKubernetesEngineManifestObject", func() {
		docs := kubernetesEngineClusterDNSManifests()
		Expect(docs).To(HaveLen(6))
		for _, doc := range docs {
			err := applyKubernetesEngineManifestObject("nonexistent-ns", "/ca", "/cert", "/key", "https://127.0.0.1:6443", []byte(doc))
			Expect(err).NotTo(MatchError(ContainSubstring("unsupported manifest resource")))
			Expect(err).NotTo(MatchError(ContainSubstring("missing metadata.name")))
		}
	})

	It("configures the kube-dns Service with the ClusterIP kubelet's clusterDNS points to", func() {
		svc := kubernetesEngineClusterDNSServiceYAML()
		Expect(svc).To(ContainSubstring("name: kube-dns"))
		Expect(svc).To(ContainSubstring("namespace: kube-system"))
		Expect(svc).To(ContainSubstring("clusterIP: 10.96.0.10"))
	})

	It("mounts the Corefile ConfigMap into the coredns Deployment", func() {
		deployment := kubernetesEngineClusterDNSDeploymentYAML()
		Expect(deployment).To(ContainSubstring("name: coredns"))
		Expect(deployment).To(ContainSubstring("serviceAccountName: coredns"))
		Expect(deployment).To(ContainSubstring(kubernetesEngineClusterDNSImage))
		Expect(deployment).To(ContainSubstring("name: coredns"))
		Expect(deployment).To(ContainSubstring("path: Corefile"))
	})

	It("uses the node's own resolv.conf to avoid a self-referencing forward loop", func() {
		deployment := kubernetesEngineClusterDNSDeploymentYAML()
		Expect(deployment).To(ContainSubstring("dnsPolicy: Default"))
	})

	It("grants the kubernetes plugin watch access to EndpointSlices", func() {
		clusterRole := kubernetesEngineClusterDNSClusterRoleYAML()
		Expect(clusterRole).To(ContainSubstring("discovery.k8s.io"))
		Expect(clusterRole).To(ContainSubstring("endpointslices"))
	})
})
