package controller

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func TestRenderKubernetesEngineNodePlaybook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.yaml")
	data := kubernetesEngineNodePlaybookData{
		NodeName:            "mke-demo-node-1",
		NodeIP:              "172.16.1.10",
		APIServerEndpoint:   "https://172.16.1.2:26443",
		KubernetesVersion:   "v1.36.2",
		ContainerdVersion:   "2.3.1",
		RuncVersion:         "1.4.0",
		CACertBase64:        "Y2E=",
		KubeletCertBase64:   "a2MtY2VydA==",
		KubeletKeyBase64:    "a2wta2V5",
		KubeProxyCertBase64: "a3AtY2VydA==",
		KubeProxyKeyBase64:  "a3Ata2V5",
	}
	if err := renderKubernetesEngineNodePlaybook(path, data); err != nil {
		t.Fatalf("renderKubernetesEngineNodePlaybook() failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"containerd-{{ containerd_version }}-linux-{{ release_arch }}.tar.gz",
		"/usr/local/bin/kubelet",
		"/usr/local/bin/kube-proxy",
		"/etc/kubernetes/pki/kubelet.crt",
		"--hostname-override=mke-demo-node-1",
		"--node-ip=172.16.1.10",
		base64.StdEncoding.EncodeToString([]byte(renderKubernetesEngineNodeKubeconfig(
			"https://172.16.1.2:26443",
			"system:node",
			"/etc/kubernetes/pki/kubelet.crt",
			"/etc/kubernetes/pki/kubelet.key",
		))),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered playbook does not contain %q", want)
		}
	}
	if strings.Contains(text, "kc-cert") || strings.Contains(text, "kl-key") {
		t.Fatalf("rendered playbook contains unencoded credential material")
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
