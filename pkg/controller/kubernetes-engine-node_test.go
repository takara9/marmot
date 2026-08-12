package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestBuildKubernetesEngineNodeServerSpec(t *testing.T) {
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
	if err != nil {
		t.Fatalf("buildKubernetesEngineNodeServerSpec() failed: %v", err)
	}
	if server.Metadata.Name != "mke-demo-node-2" {
		t.Fatalf("server name = %q, want mke-demo-node-2", server.Metadata.Name)
	}
	if server.Spec.Cpu == nil || *server.Spec.Cpu != cpu || server.Spec.Memory == nil || *server.Spec.Memory != memory {
		t.Fatalf("server resources = cpu:%v memory:%v", server.Spec.Cpu, server.Spec.Memory)
	}
	if server.Spec.NetworkInterface == nil || len(*server.Spec.NetworkInterface) != 2 {
		t.Fatalf("network interfaces = %v, want two", server.Spec.NetworkInterface)
	}
	nics := *server.Spec.NetworkInterface
	if nics[0].Networkname != "host-bridge" || nics[1].Networkname != "mke-demo" {
		t.Fatalf("network interfaces = %+v", nics)
	}
	if server.Metadata.Labels == nil || (*server.Metadata.Labels)[kubernetesEngineNodeLabelOwner] != "ke123" {
		t.Fatalf("server labels = %v", server.Metadata.Labels)
	}
	if server.Spec.Auth == nil || server.Spec.Auth.PublicKey == nil || *server.Spec.Auth.PublicKey != "ssh-rsa AAAA" {
		t.Fatalf("server auth = %+v", server.Spec.Auth)
	}
}

func TestKubernetesEngineNodesReady(t *testing.T) {
	data := []byte(`{"items":[
		{"metadata":{"name":"node-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"node-2"},"status":{"conditions":[{"type":"Ready","status":"False"}]}}
	]}`)
	ready, err := kubernetesEngineNodesReady(data, []string{"node-1", "node-2"})
	if err != nil {
		t.Fatalf("kubernetesEngineNodesReady() failed: %v", err)
	}
	if ready {
		t.Fatalf("kubernetesEngineNodesReady() = true while node-2 is not Ready")
	}

	readyData := []byte(`{"items":[
		{"metadata":{"name":"node-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"node-2"},"status":{"conditions":[{"type":"Ready","status":"True"}]}}
	]}`)
	ready, err = kubernetesEngineNodesReady(readyData, []string{"node-1", "node-2"})
	if err != nil {
		t.Fatalf("kubernetesEngineNodesReady() failed: %v", err)
	}
	if !ready {
		t.Fatalf("kubernetesEngineNodesReady() = false, want true")
	}
}

func TestKubernetesEngineNodeInternalIP(t *testing.T) {
	server := api.Server{Metadata: api.Metadata{Name: "node-1"}, Spec: api.ServerSpec{
		NetworkInterface: &[]api.NetworkInterface{
			{Networkname: "default"},
			{Networkname: "mke-demo", Address: util.StringPtr("172.16.1.10")},
		},
	}}
	address, err := kubernetesEngineNodeInternalIP(server, "mke-demo")
	if err != nil {
		t.Fatalf("kubernetesEngineNodeInternalIP() failed: %v", err)
	}
	if address != "172.16.1.10" {
		t.Fatalf("address = %q, want 172.16.1.10", address)
	}
}
