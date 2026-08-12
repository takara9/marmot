package controller

import (
	"fmt"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("runKubernetesEngineNodeProvisionSteps", func() {
	It("installs, configures and enables containerd/kubelet/kube-proxy on x86_64", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "x86_64"}}
		data := newTestKubernetesEngineNodeProvisionData()

		Expect(runKubernetesEngineNodeProvisionSteps(runner, data)).To(Succeed())

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
			Expect(joined).To(ContainSubstring(want))
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
			Expect(runner.files).To(HaveKey(path))
			if want != "" {
				Expect(runner.files[path]).To(Equal(want))
			}
		}
		Expect(runner.files["/etc/systemd/system/kubelet.service"]).To(ContainSubstring("--hostname-override=mke-demo-node-1 --node-ip=172.16.1.10"))
	})

	It("selects arm64 assets when the node reports an aarch64 architecture", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "aarch64"}}
		data := newTestKubernetesEngineNodeProvisionData()

		Expect(runKubernetesEngineNodeProvisionSteps(runner, data)).To(Succeed())

		joined := strings.Join(runner.commands, "\n")
		Expect(joined).To(ContainSubstring("containerd-2.3.1-linux-arm64.tar.gz"))
		Expect(joined).To(ContainSubstring("runc.arm64"))
	})

	It("wraps a failed step with its step name", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{
			outputs: map[string]string{"uname -m": "x86_64"},
			failCmd: "apt-get install",
		}
		data := newTestKubernetesEngineNodeProvisionData()

		err := runKubernetesEngineNodeProvisionSteps(runner, data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("install runtime dependencies"))
	})
})
