package controller

import (
	"errors"
	"reflect"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestKubernetesEngineControlPlaneNetworkNames(t *testing.T) {
	namespace, hostVeth, peerVeth, err := KubernetesEngineControlPlaneNetworkNames("demo")
	if err != nil {
		t.Fatalf("KubernetesEngineControlPlaneNetworkNames() failed: %v", err)
	}
	if namespace != "mke-demo" {
		t.Fatalf("namespace = %q, want %q", namespace, "mke-demo")
	}
	if len(hostVeth) > 15 || len(peerVeth) > 15 || hostVeth == peerVeth {
		t.Fatalf("invalid veth names: host=%q peer=%q", hostVeth, peerVeth)
	}
}

func TestCreateControlPlaneNamespaceUsesIPSubprocess(t *testing.T) {
	originalRunIP := controlPlaneRunIP
	t.Cleanup(func() { controlPlaneRunIP = originalRunIP })

	var got []string
	controlPlaneRunIP = func(args ...string) ([]byte, error) {
		got = append([]string(nil), args...)
		return nil, nil
	}

	if err := createControlPlaneNamespace("mke-demo"); err != nil {
		t.Fatalf("createControlPlaneNamespace() failed: %v", err)
	}
	want := []string{"netns", "add", "mke-demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ip args = %v, want %v", got, want)
	}
}

func TestSetupKubernetesEngineControlPlaneNetworkLifecycle(t *testing.T) {
	var calls []string
	installFakeControlPlaneNetworkOps(t, &calls)
	cfg, err := NewKubernetesEngineControlPlaneNetworkConfig("demo", "br-demo", "172.16.90.100/24")
	if err != nil {
		t.Fatalf("NewKubernetesEngineControlPlaneNetworkConfig() failed: %v", err)
	}
	if err := SetupKubernetesEngineControlPlaneNetwork(cfg); err != nil {
		t.Fatalf("SetupKubernetesEngineControlPlaneNetwork() failed: %v", err)
	}
	wantSetup := []string{"netns-add:mke-demo", "veth-add", "ovs-add", "host-up", "peer-configure"}
	if !reflect.DeepEqual(calls, wantSetup) {
		t.Fatalf("setup calls = %v, want %v", calls, wantSetup)
	}

	calls = nil
	if err := TeardownKubernetesEngineControlPlaneNetwork(cfg); err != nil {
		t.Fatalf("TeardownKubernetesEngineControlPlaneNetwork() failed: %v", err)
	}
	wantTeardown := []string{"ovs-delete", "netns-delete:mke-demo"}
	if !reflect.DeepEqual(calls, wantTeardown) {
		t.Fatalf("teardown calls = %v, want %v", calls, wantTeardown)
	}
}

func TestSetupKubernetesEngineControlPlaneNetworkRollsBack(t *testing.T) {
	var calls []string
	installFakeControlPlaneNetworkOps(t, &calls)
	controlPlaneSetHostVethUp = func(string) error {
		calls = append(calls, "host-up")
		return errors.New("link failed")
	}
	cfg, err := NewKubernetesEngineControlPlaneNetworkConfig("demo", "br-demo", "172.16.90.100/24")
	if err != nil {
		t.Fatalf("NewKubernetesEngineControlPlaneNetworkConfig() failed: %v", err)
	}
	if err := SetupKubernetesEngineControlPlaneNetwork(cfg); err == nil {
		t.Fatalf("expected setup error, got nil")
	}
	want := []string{"netns-add:mke-demo", "veth-add", "ovs-add", "host-up", "ovs-delete", "netns-delete:mke-demo"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestAllocateKubernetesEngineAPIServerPort(t *testing.T) {
	origProbe := kubernetesEnginePortProbe
	t.Cleanup(func() { kubernetesEnginePortProbe = origProbe })
	kubernetesEnginePortProbe = func(port int) bool { return port != 26444 }
	engines := []api.KubernetesEngine{{Status: &api.Status{ApiServerPort: util.IntPtrInt(26443)}}}
	port, err := AllocateKubernetesEngineAPIServerPort(engines, 26443, 26446)
	if err != nil {
		t.Fatalf("AllocateKubernetesEngineAPIServerPort() failed: %v", err)
	}
	if port != 26445 {
		t.Fatalf("port = %d, want 26445", port)
	}
}

func installFakeControlPlaneNetworkOps(t *testing.T, calls *[]string) {
	t.Helper()
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

	t.Cleanup(func() {
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
