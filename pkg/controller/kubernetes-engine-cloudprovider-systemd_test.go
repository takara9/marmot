package controller

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineCloudControllerManagerUnit", func() {
	It("derives the systemd unit name from the cluster name", func() {
		Expect(KubernetesEngineCloudControllerManagerUnitName("demo")).To(Equal("mke-cloud-controller-manager-demo.service"))
	})

	It("renders the kubeconfig path and extra args into the unit file", func() {
		content := renderKubernetesEngineCloudControllerManagerUnit(KubernetesEngineCloudControllerManagerUnitConfig{
			ClusterName:    "demo",
			BinaryPath:     "/bin/mke-node-controller",
			KubeconfigPath: "/config/cloud-controller-manager.kubeconfig",
			ExtraArgs:      []string{"--v=2"},
		})
		for _, want := range []string{
			"Requires=mke-kube-apiserver-demo.service",
			"--kubeconfig=/config/cloud-controller-manager.kubeconfig",
			"--v=2",
		} {
			Expect(content).To(ContainSubstring(want))
		}
		Expect(content).NotTo(ContainSubstring("NetworkNamespacePath"))
	})

	It("creates and deletes the cloud-controller-manager unit", func() {
		recorder := installFakeSystemd()
		origUnitDir := controlPlaneSystemdUnitDir
		controlPlaneSystemdUnitDir = GinkgoT().TempDir()
		DeferCleanup(func() { controlPlaneSystemdUnitDir = origUnitDir })

		cfg := KubernetesEngineCloudControllerManagerUnitConfig{
			ClusterName:    "demo",
			BinaryPath:     "/bin/cloud-controller-manager",
			KubeconfigPath: "/config/cloud-controller-manager.kubeconfig",
		}
		Expect(CreateKubernetesEngineCloudControllerManagerUnit(cfg)).To(Succeed())
		Expect(recorder.calls).To(Equal([]string{
			"daemon-reload",
			"enable:mke-cloud-controller-manager-demo.service",
			"start:mke-cloud-controller-manager-demo.service",
		}))

		unitPath := filepath.Join(controlPlaneSystemdUnitDir, "mke-cloud-controller-manager-demo.service")
		_, err := os.ReadFile(unitPath)
		Expect(err).NotTo(HaveOccurred())

		recorder.calls = nil
		Expect(DeleteKubernetesEngineCloudControllerManagerUnit("demo")).To(Succeed())
		Expect(recorder.calls).To(Equal([]string{
			"stop:mke-cloud-controller-manager-demo.service",
			"disable:mke-cloud-controller-manager-demo.service",
			"daemon-reload",
		}))
		_, err = os.ReadFile(unitPath)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("treats deletion of a never-created unit as a success", func() {
		installFakeSystemd()
		origUnitDir := controlPlaneSystemdUnitDir
		controlPlaneSystemdUnitDir = GinkgoT().TempDir()
		DeferCleanup(func() { controlPlaneSystemdUnitDir = origUnitDir })

		Expect(DeleteKubernetesEngineCloudControllerManagerUnit("demo")).To(Succeed())
	})
})
