package controller

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var kubernetesEngineNodeSSHPort = 22

var kubernetesEngineNodeSSHDialTimeout = 10 * time.Second

// kubernetesEngineNodeProvisionData holds everything needed to provision a
// KubernetesEngine worker node over SSH (replacing the former ansible-playbook based flow).
type kubernetesEngineNodeProvisionData struct {
	NodeName            string
	NodeIP              string
	KubernetesVersion   string
	ContainerdVersion   string
	RuncVersion         string
	CACert              []byte
	KubeletCert         []byte
	KubeletKey          []byte
	KubeProxyCert       []byte
	KubeProxyKey        []byte
	KubeletKubeconfig   []byte
	KubeProxyKubeconfig []byte
	KubeletConfig       []byte
	KubeProxyConfig     []byte
}

// kubernetesEngineNodeCommandRunner executes commands on a KubernetesEngine node,
// allowing the provisioning steps to be tested without a real SSH connection.
type kubernetesEngineNodeCommandRunner interface {
	step(name string, fn func() error) error
	run(cmd string, stdin io.Reader) error
	output(cmd string) (string, error)
	writeFile(path, mode string, content []byte) error
}

// provisionKubernetesEngineNodeSSH connects to the node via SSH and runs the same setup
// steps that were previously encoded as an ansible-playbook (install runtime deps, fetch
// containerd/runc/kubelet/kube-proxy, place credentials, install systemd units).
func provisionKubernetesEngineNodeSSH(address, privateKeyPath, nodeID string, data kubernetesEngineNodeProvisionData) error {
	client, err := dialKubernetesEngineNodeSSH(address, privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", data.NodeName, err)
	}
	defer func() { _ = client.Close() }()

	runner := &kubernetesEngineNodeSSHRunner{client: client, resourceID: nodeID}
	return runKubernetesEngineNodeProvisionSteps(runner, data)
}

func runKubernetesEngineNodeProvisionSteps(runner kubernetesEngineNodeCommandRunner, data kubernetesEngineNodeProvisionData) error {

	if err := runner.step("install runtime dependencies", func() error {
		return runner.run("DEBIAN_FRONTEND=noninteractive apt-get update && "+
			"DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates conntrack curl iptables socat", nil)
	}); err != nil {
		return err
	}

	arch, err := detectKubernetesEngineNodeArch(runner)
	if err != nil {
		return fmt.Errorf("failed to detect node architecture: %w", err)
	}

	if err := runner.step("create Kubernetes directories", func() error {
		return runner.run("mkdir -p /etc/kubernetes /etc/kubernetes/pki /etc/kubernetes/kubelet "+
			"/etc/kubernetes/kube-proxy /opt/cni/bin", nil)
	}); err != nil {
		return err
	}

	containerdURL := fmt.Sprintf("https://github.com/containerd/containerd/releases/download/v%s/containerd-%s-linux-%s.tar.gz",
		data.ContainerdVersion, data.ContainerdVersion, arch)
	if err := runner.step("install containerd", func() error {
		return runner.run(fmt.Sprintf("test -e /usr/local/bin/containerd || (curl -fsSL %s | tar -xz -C /usr/local)",
			shellQuote(containerdURL)), nil)
	}); err != nil {
		return err
	}

	runcURL := fmt.Sprintf("https://github.com/opencontainers/runc/releases/download/v%s/runc.%s", data.RuncVersion, arch)
	if err := runner.step("install runc", func() error {
		return runner.run(fmt.Sprintf("test -e /usr/local/sbin/runc || (curl -fsSL %s -o /usr/local/sbin/runc && chmod 0755 /usr/local/sbin/runc)",
			shellQuote(runcURL)), nil)
	}); err != nil {
		return err
	}

	for _, bin := range []string{"kubelet", "kube-proxy"} {
		binURL := fmt.Sprintf("https://dl.k8s.io/release/%s/bin/linux/%s/%s", data.KubernetesVersion, arch, bin)
		dest := "/usr/local/bin/" + bin
		if err := runner.step("install "+bin+" binary", func() error {
			return runner.run(fmt.Sprintf("test -e %s || (curl -fsSL %s -o %s && chmod 0755 %s)",
				shellQuote(dest), shellQuote(binURL), shellQuote(dest), shellQuote(dest)), nil)
		}); err != nil {
			return err
		}
	}

	credentialFiles := []struct {
		path    string
		content []byte
	}{
		{"/etc/kubernetes/pki/ca.crt", data.CACert},
		{"/etc/kubernetes/pki/kubelet.crt", data.KubeletCert},
		{"/etc/kubernetes/pki/kubelet.key", data.KubeletKey},
		{"/etc/kubernetes/pki/kube-proxy.crt", data.KubeProxyCert},
		{"/etc/kubernetes/pki/kube-proxy.key", data.KubeProxyKey},
		{"/etc/kubernetes/kubelet.kubeconfig", data.KubeletKubeconfig},
		{"/etc/kubernetes/kube-proxy.kubeconfig", data.KubeProxyKubeconfig},
		{"/etc/kubernetes/kubelet/config.yaml", data.KubeletConfig},
		{"/etc/kubernetes/kube-proxy/config.yaml", data.KubeProxyConfig},
	}
	if err := runner.step("install cluster credentials and configuration", func() error {
		for _, file := range credentialFiles {
			if err := runner.writeFile(file.path, "0600", file.content); err != nil {
				return fmt.Errorf("failed to write %s: %w", file.path, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runner.step("generate containerd configuration", func() error {
		return runner.run("mkdir -p /etc/containerd && "+
			"test -e /etc/containerd/config.toml || "+
			"(/usr/local/bin/containerd config default > /etc/containerd/config.toml && "+
			"sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml)", nil)
	}); err != nil {
		return err
	}

	units := []struct {
		name    string
		content string
	}{
		{"containerd.service", kubernetesEngineNodeContainerdUnit()},
		{"kubelet.service", kubernetesEngineNodeKubeletUnit(data.NodeName, data.NodeIP)},
		{"kube-proxy.service", kubernetesEngineNodeKubeProxyUnit()},
	}
	if err := runner.step("install systemd units", func() error {
		for _, unit := range units {
			path := "/etc/systemd/system/" + unit.name
			if err := runner.writeFile(path, "0644", []byte(unit.content)); err != nil {
				return fmt.Errorf("failed to write %s: %w", path, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return runner.step("enable Kubernetes node services", func() error {
		return runner.run("systemctl daemon-reload && systemctl enable --now containerd kubelet kube-proxy", nil)
	})
}

func detectKubernetesEngineNodeArch(runner kubernetesEngineNodeCommandRunner) (string, error) {
	machine, err := runner.output("uname -m")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(machine) == "x86_64" {
		return "amd64", nil
	}
	return "arm64", nil
}

func kubernetesEngineNodeContainerdUnit() string {
	return `[Unit]
Description=containerd container runtime
After=network.target local-fs.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/containerd
Restart=always
Delegate=yes
KillMode=process

[Install]
WantedBy=multi-user.target
`
}

func kubernetesEngineNodeKubeletUnit(nodeName, nodeIP string) string {
	return fmt.Sprintf(`[Unit]
Description=Kubernetes Kubelet
Wants=network-online.target containerd.service
After=network-online.target containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet --config=/etc/kubernetes/kubelet/config.yaml --kubeconfig=/etc/kubernetes/kubelet.kubeconfig --hostname-override=%s --node-ip=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, nodeName, nodeIP)
}

func kubernetesEngineNodeKubeProxyUnit() string {
	return `[Unit]
Description=Kubernetes Kube Proxy
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/local/bin/kube-proxy --config=/etc/kubernetes/kube-proxy/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
}

func dialKubernetesEngineNodeSSH(address, privateKeyPath string) (*ssh.Client, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// ノードは使い捨てのVMであり、事前にホスト鍵を検証する手段がないため無視する
		// （旧ansible実行時の -o StrictHostKeyChecking=no と同等）。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         kubernetesEngineNodeSSHDialTimeout,
	}
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return nil, fmt.Errorf("node address is empty")
	}
	return ssh.Dial("tcp", net.JoinHostPort(trimmed, fmt.Sprintf("%d", kubernetesEngineNodeSSHPort)), config)
}

// kubernetesEngineNodeSSHRunner executes commands on a KubernetesEngine node over SSH,
// streaming each output line to slog so long-running steps stay visible in CI logs.
type kubernetesEngineNodeSSHRunner struct {
	client     *ssh.Client
	resourceID string
}

func (r *kubernetesEngineNodeSSHRunner) step(name string, fn func() error) error {
	slog.Info("kubernetes-engine-node provision step", "id", r.resourceID, "step", name)
	if err := fn(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (r *kubernetesEngineNodeSSHRunner) run(cmd string, stdin io.Reader) error {
	session, err := r.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to open ssh session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var buf bytes.Buffer
	lineLogger := &kubernetesEngineNodeSSHLineLogger{resourceID: r.resourceID}
	mw := io.MultiWriter(&buf, lineLogger)
	session.Stdout = mw
	session.Stderr = mw
	if stdin != nil {
		session.Stdin = stdin
	}
	if err := session.Run(cmd); err != nil {
		trimmed := strings.TrimSpace(buf.String())
		if trimmed == "" {
			return fmt.Errorf("remote command failed: %w", err)
		}
		return fmt.Errorf("remote command failed: %w: %s", err, trimmed)
	}
	return nil
}

func (r *kubernetesEngineNodeSSHRunner) output(cmd string) (string, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to open ssh session: %w", err)
	}
	defer func() { _ = session.Close() }()
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("remote command failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *kubernetesEngineNodeSSHRunner) writeFile(path, mode string, content []byte) error {
	cmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod %s %s",
		shellQuote(parentDir(path)), shellQuote(path), mode, shellQuote(path))
	return r.run(cmd, bytes.NewReader(content))
}

func parentDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return "/"
	}
	return path[:idx]
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// kubernetesEngineNodeSSHLineLogger streams remote command output to slog line-by-line,
// mirroring the ansible-playbook progress logging it replaces.
type kubernetesEngineNodeSSHLineLogger struct {
	resourceID string
	pending    []byte
}

func (l *kubernetesEngineNodeSSHLineLogger) Write(p []byte) (int, error) {
	l.pending = append(l.pending, p...)
	for {
		idx := bytes.IndexByte(l.pending, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(l.pending[:idx]), "\r")
		l.pending = l.pending[idx+1:]
		if line == "" {
			continue
		}
		slog.Info("kubernetes-engine-node provision progress", "id", l.resourceID, "line", line)
	}
	return len(p), nil
}
