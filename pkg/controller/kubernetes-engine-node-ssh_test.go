package controller

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

// fakeKubernetesEngineNodeCommandRunner records every command/file write issued by
// runKubernetesEngineNodeProvisionSteps so it can be verified without a real SSH connection.
type fakeKubernetesEngineNodeCommandRunner struct {
	commands []string
	files    map[string]string
	outputs  map[string]string
	failCmd  string
}

func (r *fakeKubernetesEngineNodeCommandRunner) step(name string, fn func() error) error {
	if err := fn(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (r *fakeKubernetesEngineNodeCommandRunner) run(cmd string, stdin io.Reader) error {
	r.commands = append(r.commands, cmd)
	if r.failCmd != "" && strings.Contains(cmd, r.failCmd) {
		return fmt.Errorf("simulated failure")
	}
	return nil
}

func (r *fakeKubernetesEngineNodeCommandRunner) output(cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	return r.outputs[cmd], nil
}

func (r *fakeKubernetesEngineNodeCommandRunner) writeFile(path, mode string, content []byte) error {
	if r.files == nil {
		r.files = map[string]string{}
	}
	r.files[path] = string(content)
	return nil
}

func newTestKubernetesEngineNodeProvisionData() kubernetesEngineNodeProvisionData {
	return kubernetesEngineNodeProvisionData{
		NodeName:            "mke-demo-node-1",
		NodeIP:              "172.16.1.10",
		KubernetesVersion:   "v1.36.2",
		ContainerdVersion:   "2.3.1",
		RuncVersion:         "1.4.0",
		CACert:              []byte("ca"),
		KubeletCert:         []byte("kubelet-cert"),
		KubeletKey:          []byte("kubelet-key"),
		KubeProxyCert:       []byte("kube-proxy-cert"),
		KubeProxyKey:        []byte("kube-proxy-key"),
		KubeletKubeconfig:   []byte("kubelet-kubeconfig"),
		KubeProxyKubeconfig: []byte("kube-proxy-kubeconfig"),
		KubeletConfig:       []byte("kubelet-config"),
		KubeProxyConfig:     []byte("kube-proxy-config"),
	}
}

func TestRunKubernetesEngineNodeProvisionSteps(t *testing.T) {
	runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "x86_64"}}
	data := newTestKubernetesEngineNodeProvisionData()

	if err := runKubernetesEngineNodeProvisionSteps(runner, data); err != nil {
		t.Fatalf("runKubernetesEngineNodeProvisionSteps() failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"apt-get install -y ca-certificates conntrack curl iptables socat",
		"uname -m",
		"mkdir -p /etc/kubernetes /etc/kubernetes/pki /etc/kubernetes/kubelet /etc/kubernetes/kube-proxy /opt/cni/bin",
		"containerd-2.3.1-linux-amd64.tar.gz",
		"runc.amd64",
		"/usr/local/bin/kubelet",
		"/usr/local/bin/kube-proxy",
		"containerd config default > /etc/containerd/config.toml",
		"systemctl daemon-reload && systemctl enable --now containerd kubelet kube-proxy",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("commands do not contain %q; got: %s", want, joined)
		}
	}

	wantFiles := map[string]string{
		"/etc/kubernetes/pki/ca.crt":             "ca",
		"/etc/kubernetes/pki/kubelet.crt":        "kubelet-cert",
		"/etc/kubernetes/pki/kubelet.key":        "kubelet-key",
		"/etc/kubernetes/pki/kube-proxy.crt":     "kube-proxy-cert",
		"/etc/kubernetes/pki/kube-proxy.key":     "kube-proxy-key",
		"/etc/kubernetes/kubelet.kubeconfig":     "kubelet-kubeconfig",
		"/etc/kubernetes/kube-proxy.kubeconfig":  "kube-proxy-kubeconfig",
		"/etc/kubernetes/kubelet/config.yaml":    "kubelet-config",
		"/etc/kubernetes/kube-proxy/config.yaml": "kube-proxy-config",
		"/etc/systemd/system/containerd.service": "",
		"/etc/systemd/system/kubelet.service":    "",
		"/etc/systemd/system/kube-proxy.service": "",
	}
	for path, want := range wantFiles {
		got, ok := runner.files[path]
		if !ok {
			t.Fatalf("file %s was not written", path)
		}
		if want != "" && got != want {
			t.Fatalf("file %s content = %q, want %q", path, got, want)
		}
	}
	if !strings.Contains(runner.files["/etc/systemd/system/kubelet.service"], "--hostname-override=mke-demo-node-1 --node-ip=172.16.1.10") {
		t.Fatalf("kubelet.service does not reference node name/ip: %s", runner.files["/etc/systemd/system/kubelet.service"])
	}
}

func TestRunKubernetesEngineNodeProvisionSteps_ArmArch(t *testing.T) {
	runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "aarch64"}}
	data := newTestKubernetesEngineNodeProvisionData()

	if err := runKubernetesEngineNodeProvisionSteps(runner, data); err != nil {
		t.Fatalf("runKubernetesEngineNodeProvisionSteps() failed: %v", err)
	}

	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "containerd-2.3.1-linux-arm64.tar.gz") || !strings.Contains(joined, "runc.arm64") {
		t.Fatalf("commands do not reflect arm64 architecture: %s", joined)
	}
}

func TestRunKubernetesEngineNodeProvisionSteps_StepFailureIsWrapped(t *testing.T) {
	runner := &fakeKubernetesEngineNodeCommandRunner{
		outputs: map[string]string{"uname -m": "x86_64"},
		failCmd: "apt-get install",
	}
	data := newTestKubernetesEngineNodeProvisionData()

	err := runKubernetesEngineNodeProvisionSteps(runner, data)
	if err == nil {
		t.Fatalf("runKubernetesEngineNodeProvisionSteps() succeeded, want error")
	}
	if !strings.Contains(err.Error(), "install runtime dependencies") {
		t.Fatalf("error = %v, want it to mention the failed step name", err)
	}
}
