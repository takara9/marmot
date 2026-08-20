package controller

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubernetesEngineControlPlaneUnits", func() {
	It("creates and deletes the control-plane systemd units", func() {
		recorder := installFakeSystemd()
		origUnitDir := controlPlaneSystemdUnitDir
		controlPlaneSystemdUnitDir = GinkgoT().TempDir()
		DeferCleanup(func() { controlPlaneSystemdUnitDir = origUnitDir })

		cfg := testControlPlaneUnitConfig()
		Expect(CreateKubernetesEngineControlPlaneUnits(cfg)).To(Succeed())
		wantCreate := []string{
			"daemon-reload",
			"enable:mke-kube-apiserver-demo.service", "start:mke-kube-apiserver-demo.service",
			"enable:mke-kube-scheduler-demo.service", "start:mke-kube-scheduler-demo.service",
			"enable:mke-kube-controller-manager-demo.service", "start:mke-kube-controller-manager-demo.service",
		}
		Expect(recorder.calls).To(Equal(wantCreate))

		apiServerUnit, err := os.ReadFile(filepath.Join(controlPlaneSystemdUnitDir, "mke-kube-apiserver-demo.service"))
		Expect(err).NotTo(HaveOccurred())
		for _, want := range []string{"NetworkNamespacePath=/run/netns/mke-demo", "--etcd-servers=http://127.0.0.1:23790", "--secure-port=6443", "--advertise-address=172.16.90.100", "--kubelet-preferred-address-types=InternalIP", "--kubelet-client-certificate=/pki/kube-apiserver-kubelet-client.crt", "--kubelet-client-key=/pki/kube-apiserver-kubelet-client.key"} {
			Expect(string(apiServerUnit)).To(ContainSubstring(want))
		}

		recorder.calls = nil
		Expect(DeleteKubernetesEngineControlPlaneUnits("demo")).To(Succeed())
		wantDelete := []string{
			"stop:mke-kube-controller-manager-demo.service", "disable:mke-kube-controller-manager-demo.service",
			"stop:mke-kube-scheduler-demo.service", "disable:mke-kube-scheduler-demo.service",
			"stop:mke-kube-apiserver-demo.service", "disable:mke-kube-apiserver-demo.service",
			"daemon-reload",
		}
		Expect(recorder.calls).To(Equal(wantDelete))
	})

	It("checks control-plane health using the configured namespace and endpoint", func() {
		origCommand := controlPlaneHealthCommand
		DeferCleanup(func() { controlPlaneHealthCommand = origCommand })
		var gotNamespace, gotCAPath, gotEndpoint string
		controlPlaneHealthCommand = func(_ context.Context, namespace, caPath, endpoint string) error {
			gotNamespace, gotCAPath, gotEndpoint = namespace, caPath, endpoint
			return nil
		}
		Expect(CheckKubernetesEngineControlPlaneHealth("mke-demo", "/pki/ca.crt", "172.16.90.100", 6443)).To(Succeed())
		Expect(gotNamespace).To(Equal("mke-demo"))
		Expect(gotCAPath).To(Equal("/pki/ca.crt"))
		Expect(gotEndpoint).To(Equal("https://172.16.90.100:6443/healthz"))
	})
})

func testControlPlaneUnitConfig() KubernetesEngineControlPlaneUnitConfig {
	return KubernetesEngineControlPlaneUnitConfig{
		ClusterName:        "demo",
		NetworkNamespace:   "mke-demo",
		APIServerIP:        "172.16.90.100",
		APIServerPort:      6443,
		EtcdClientPort:     23790,
		ServiceClusterCIDR: "10.96.0.0/12",
		Binaries: map[string]string{
			"kube-apiserver":          "/bin/kube-apiserver",
			"kube-scheduler":          "/bin/kube-scheduler",
			"kube-controller-manager": "/bin/kube-controller-manager",
		},
		Assets: KubernetesEngineControlPlaneAssets{
			CACertPath:                   "/pki/ca.crt",
			APIServerCertPath:            "/pki/kube-apiserver.crt",
			APIServerKeyPath:             "/pki/kube-apiserver.key",
			KubeletClientCertPath:        "/pki/kube-apiserver-kubelet-client.crt",
			KubeletClientKeyPath:         "/pki/kube-apiserver-kubelet-client.key",
			SchedulerKubeconfigPath:      "/config/kube-scheduler.kubeconfig",
			ControllerManagerConfigPath:  "/config/kube-controller-manager.kubeconfig",
			ServiceAccountPublicKeyPath:  "/pki/service-account.pub",
			ServiceAccountPrivateKeyPath: "/pki/service-account.key",
		},
	}
}
