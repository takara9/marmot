package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("kubernetesEngineCloudControllerManagerRBACManifests", func() {
	It("returns manifests supported by applyKubernetesEngineManifestObject", func() {
		docs := kubernetesEngineCloudControllerManagerRBACManifests()
		Expect(docs).To(HaveLen(2))
		for _, doc := range docs {
			err := applyKubernetesEngineManifestObject("nonexistent-ns", "/ca", "/cert", "/key", "https://127.0.0.1:6443", []byte(doc))
			Expect(err).NotTo(MatchError(ContainSubstring("unsupported manifest resource")))
			Expect(err).NotTo(MatchError(ContainSubstring("missing metadata.name")))
		}
	})

	It("grants nodes/services access required by cmd/mke-node-controller's reconcile loop", func() {
		clusterRole := kubernetesEngineCloudControllerManagerClusterRoleYAML()
		Expect(clusterRole).To(ContainSubstring("name: system:cloud-controller-manager"))
		Expect(clusterRole).To(ContainSubstring(`resources: ["nodes"]`))
		Expect(clusterRole).To(ContainSubstring(`resources: ["nodes/status"]`))
		Expect(clusterRole).To(ContainSubstring(`resources: ["services"]`))
		Expect(clusterRole).To(ContainSubstring(`resources: ["services/status"]`))
	})

	It("binds the ClusterRole to the cloud-controller-manager client certificate identity", func() {
		binding := kubernetesEngineCloudControllerManagerClusterRoleBindingYAML()
		Expect(binding).To(ContainSubstring("name: system:cloud-controller-manager"))
		Expect(binding).To(ContainSubstring("kind: User"))
		Expect(binding).To(ContainSubstring("name: " + kubernetesEngineCloudControllerManagerUserName))
	})
})
