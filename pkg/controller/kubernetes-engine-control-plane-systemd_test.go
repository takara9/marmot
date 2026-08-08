package controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKubernetesEngineControlPlaneUnitLifecycle(t *testing.T) {
	recorder := installFakeSystemd(t)
	origUnitDir := controlPlaneSystemdUnitDir
	controlPlaneSystemdUnitDir = t.TempDir()
	t.Cleanup(func() { controlPlaneSystemdUnitDir = origUnitDir })

	cfg := testControlPlaneUnitConfig()
	if err := CreateKubernetesEngineControlPlaneUnits(cfg); err != nil {
		t.Fatalf("CreateKubernetesEngineControlPlaneUnits() failed: %v", err)
	}
	wantCreate := []string{
		"daemon-reload",
		"enable:mke-kube-apiserver-demo.service", "start:mke-kube-apiserver-demo.service",
		"enable:mke-kube-scheduler-demo.service", "start:mke-kube-scheduler-demo.service",
		"enable:mke-kube-controller-manager-demo.service", "start:mke-kube-controller-manager-demo.service",
	}
	assertCallsEqual(t, recorder.calls, wantCreate)

	apiServerUnit, err := os.ReadFile(filepath.Join(controlPlaneSystemdUnitDir, "mke-kube-apiserver-demo.service"))
	if err != nil {
		t.Fatalf("failed to read API server unit: %v", err)
	}
	for _, want := range []string{"NetworkNamespacePath=/run/netns/mke-demo", "--etcd-servers=http://127.0.0.1:23790", "--secure-port=6443", "--advertise-address=172.16.90.100"} {
		if !strings.Contains(string(apiServerUnit), want) {
			t.Fatalf("API server unit does not contain %q: %s", want, apiServerUnit)
		}
	}

	recorder.calls = nil
	if err := DeleteKubernetesEngineControlPlaneUnits("demo"); err != nil {
		t.Fatalf("DeleteKubernetesEngineControlPlaneUnits() failed: %v", err)
	}
	wantDelete := []string{
		"stop:mke-kube-controller-manager-demo.service", "disable:mke-kube-controller-manager-demo.service",
		"stop:mke-kube-scheduler-demo.service", "disable:mke-kube-scheduler-demo.service",
		"stop:mke-kube-apiserver-demo.service", "disable:mke-kube-apiserver-demo.service",
		"daemon-reload",
	}
	assertCallsEqual(t, recorder.calls, wantDelete)
}

func TestCheckKubernetesEngineControlPlaneHealth(t *testing.T) {
	origCommand := controlPlaneHealthCommand
	t.Cleanup(func() { controlPlaneHealthCommand = origCommand })
	controlPlaneHealthCommand = func(_ context.Context, namespace, caPath, endpoint string) error {
		if namespace != "mke-demo" || caPath != "/pki/ca.crt" || endpoint != "https://172.16.90.100:6443/healthz" {
			t.Fatalf("unexpected health check args: %q %q %q", namespace, caPath, endpoint)
		}
		return nil
	}
	if err := CheckKubernetesEngineControlPlaneHealth("mke-demo", "/pki/ca.crt", "172.16.90.100", 6443); err != nil {
		t.Fatalf("CheckKubernetesEngineControlPlaneHealth() failed: %v", err)
	}
}

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
			SchedulerKubeconfigPath:      "/config/kube-scheduler.kubeconfig",
			ControllerManagerConfigPath:  "/config/kube-controller-manager.kubeconfig",
			ServiceAccountPublicKeyPath:  "/pki/service-account.pub",
			ServiceAccountPrivateKeyPath: "/pki/service-account.key",
		},
	}
}
