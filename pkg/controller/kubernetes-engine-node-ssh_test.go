package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// writeTestSSHPrivateKey writes a freshly generated RSA private key (PKCS#1 PEM, the
// format ssh.ParsePrivateKey accepts) to dir and returns its path.
func writeTestSSHPrivateKey(dir string) string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	path := filepath.Join(dir, "id_rsa")
	Expect(os.WriteFile(path, pem.EncodeToMemory(block), 0o600)).To(Succeed())
	return path
}

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

var _ = Describe("dialKubernetesEngineNodeSSH", func() {
	It("routes the TCP dial through the requested network namespace", func() {
		origDial := kubernetesEngineNodeDialTCP
		DeferCleanup(func() { kubernetesEngineNodeDialTCP = origDial })

		clientConn, serverConn := net.Pipe()
		// サーバー側を即座に閉じることで、本物のSSHサーバーが応答しない状態でも
		// ハンドシェイクが無限に待たされず速やかに失敗するようにする。
		Expect(serverConn.Close()).To(Succeed())

		var gotNamespace, gotNetwork, gotAddress string
		kubernetesEngineNodeDialTCP = func(namespace, network, address string) (net.Conn, error) {
			gotNamespace, gotNetwork, gotAddress = namespace, network, address
			return clientConn, nil
		}

		keyPath := writeTestSSHPrivateKey(GinkgoT().TempDir())

		// 相手側に本物のSSHサーバーが無いためハンドシェイクは失敗するが、
		// namespace/network/addressがダイヤル層へ正しく伝わることは検証できる。
		_, err := dialKubernetesEngineNodeSSH("172.16.1.10", keyPath, "mke-demo")
		Expect(err).To(HaveOccurred())
		Expect(gotNamespace).To(Equal("mke-demo"))
		Expect(gotNetwork).To(Equal("tcp"))
		Expect(gotAddress).To(Equal("172.16.1.10:22"))
	})

	It("dials directly when no namespace is given", func() {
		origDial := kubernetesEngineNodeDialTCP
		DeferCleanup(func() { kubernetesEngineNodeDialTCP = origDial })

		var gotNamespace string
		called := false
		kubernetesEngineNodeDialTCP = func(namespace, network, address string) (net.Conn, error) {
			called = true
			gotNamespace = namespace
			return nil, fmt.Errorf("simulated dial failure")
		}

		keyPath := writeTestSSHPrivateKey(GinkgoT().TempDir())
		_, err := dialKubernetesEngineNodeSSH("172.16.1.10", keyPath, "")
		Expect(err).To(HaveOccurred())
		Expect(called).To(BeTrue())
		Expect(gotNamespace).To(BeEmpty())
	})
})

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
		NetworkKind:         kubernetesEngineNetworkKindBridge,
		PodCIDR:             "10.244.1.0/24",
		PodNetworkSupernet:  kubernetesEnginePodNetworkSupernet,
		CNIPluginsVersion:   "1.4.0",
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
			"modprobe br_netfilter",
			"net.bridge.bridge-nf-call-iptables=1",
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
		Expect(runner.files["/etc/systemd/system/kubelet.service"]).NotTo(ContainSubstring("--cloud-provider=external"))
	})

	It("adds --cloud-provider=external to the kubelet unit when CloudProviderEnabled is set", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "x86_64"}}
		data := newTestKubernetesEngineNodeProvisionData()
		data.CloudProviderEnabled = true

		Expect(runKubernetesEngineNodeProvisionSteps(runner, data)).To(Succeed())

		Expect(runner.files["/etc/systemd/system/kubelet.service"]).To(ContainSubstring("--hostname-override=mke-demo-node-1 --node-ip=172.16.1.10 --cloud-provider=external"))
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

	It("installs and configures the bridge CNI when network.kind is not cilium", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "x86_64"}}
		data := newTestKubernetesEngineNodeProvisionData()

		Expect(runKubernetesEngineNodeProvisionSteps(runner, data)).To(Succeed())

		joined := strings.Join(runner.commands, "\n")
		Expect(joined).To(ContainSubstring("cni-plugins-linux-amd64-v1.4.0.tgz"))
		Expect(joined).To(ContainSubstring("iptables -t nat"))
		Expect(runner.files).To(HaveKey("/etc/cni/net.d/10-bridge.conflist"))
		Expect(runner.files["/etc/cni/net.d/10-bridge.conflist"]).To(ContainSubstring(`"subnet": "10.244.1.0/24"`))
	})

	It("persists the pod network NAT rules so a node reboot does not lose them", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "x86_64"}}
		data := newTestKubernetesEngineNodeProvisionData()

		Expect(runKubernetesEngineNodeProvisionSteps(runner, data)).To(Succeed())

		joined := strings.Join(runner.commands, "\n")
		Expect(joined).To(ContainSubstring("systemctl daemon-reload && systemctl enable marmot-mke-iptables-restore.service"))
		Expect(joined).To(ContainSubstring("/usr/local/sbin/marmot-mke-iptables-restore.sh save"))
		Expect(runner.files).To(HaveKey("/usr/local/sbin/marmot-mke-iptables-restore.sh"))
		Expect(runner.files["/usr/local/sbin/marmot-mke-iptables-restore.sh"]).To(ContainSubstring("iptables-save > \"${RULES_FILE}\""))
		Expect(runner.files).To(HaveKey("/etc/systemd/system/marmot-mke-iptables-restore.service"))
		Expect(runner.files["/etc/systemd/system/marmot-mke-iptables-restore.service"]).To(ContainSubstring("ExecStart=/usr/local/sbin/marmot-mke-iptables-restore.sh"))
		Expect(runner.files["/etc/systemd/system/marmot-mke-iptables-restore.service"]).To(ContainSubstring("ExecStop=/usr/local/sbin/marmot-mke-iptables-restore.sh save"))
	})

	It("skips the bridge CNI installation when network.kind is cilium", func() {
		runner := &fakeKubernetesEngineNodeCommandRunner{outputs: map[string]string{"uname -m": "x86_64"}}
		data := newTestKubernetesEngineNodeProvisionData()
		data.NetworkKind = kubernetesEngineNetworkKindCilium

		Expect(runKubernetesEngineNodeProvisionSteps(runner, data)).To(Succeed())

		joined := strings.Join(runner.commands, "\n")
		Expect(joined).NotTo(ContainSubstring("cni-plugins-linux"))
		Expect(runner.files).NotTo(HaveKey("/etc/cni/net.d/10-bridge.conflist"))
		Expect(runner.files).NotTo(HaveKey("/usr/local/sbin/marmot-mke-iptables-restore.sh"))
	})
})

var _ = Describe("kubernetesEnginePodNetworkNATScript", func() {
	It("exempts pod-to-pod traffic and masquerades traffic leaving the pod network", func() {
		script := kubernetesEnginePodNetworkNATScript("10.244.0.0/16")
		Expect(script).To(ContainSubstring("-s 10.244.0.0/16 -d 10.244.0.0/16 -j RETURN"))
		Expect(script).To(ContainSubstring("-s 10.244.0.0/16 ! -d 10.244.0.0/16 -j MASQUERADE"))
		Expect(script).To(ContainSubstring("iptables -t nat -C POSTROUTING"))
	})
})

var _ = Describe("applyKubernetesEngineNodeRoutes", func() {
	It("does nothing when there are no routes to apply", func() {
		Expect(applyKubernetesEngineNodeRoutes("172.16.1.10", "/nonexistent/key", "", "node-1", nil)).To(Succeed())
	})

	It("fails to dial when the private key cannot be read", func() {
		routes := []kubernetesEngineNodeRoute{{CIDR: "10.244.2.0/24", Via: "172.16.1.11"}}
		err := applyKubernetesEngineNodeRoutes("172.16.1.10", "/nonexistent/key", "", "node-1", routes)
		Expect(err).To(HaveOccurred())
	})
})
