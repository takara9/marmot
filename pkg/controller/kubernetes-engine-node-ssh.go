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
	NodeName          string
	NodeIP            string
	KubernetesVersion string
	ContainerdVersion string
	RuncVersion       string
	// ForceBinaryReplace は、kubelet/kube-proxyバイナリが既に存在してもKubernetesVersionの内容で
	// 強制的に再ダウンロード・再インストールし、サービスを再起動する(バージョンアップグレード時に使用)。
	// falseの場合は既存動作(test -eで存在すればスキップ)を維持する。
	ForceBinaryReplace  bool
	CACert              []byte
	KubeletCert         []byte
	KubeletKey          []byte
	KubeProxyCert       []byte
	KubeProxyKey        []byte
	KubeletKubeconfig   []byte
	KubeProxyKubeconfig []byte
	KubeletConfig       []byte
	KubeProxyConfig     []byte

	// NetworkKind は spec.nodeSpec.network.cni-plugin ("none"=bridge または "cilium")。
	// "cilium" の場合、ノード側でのBridge CNIのインストール・設定は行わない
	// （Ciliumのインストールマニフェスト側がCNI設定を配置する）。
	NetworkKind string
	// PodCIDR は、このノード専用のPod CIDR（例: 10.244.1.0/24）。Bridge CNI選択時のみ使用。
	PodCIDR string
	// PodNetworkSupernet は、全ノードのPod CIDRを包含するスーパーネット。
	// Bridge CNI選択時、この範囲宛の通信はマスカレードせず直接ルーティングする。
	PodNetworkSupernet string
	// CNIPluginsVersion は containernetworking/plugins のリリースバージョン。
	CNIPluginsVersion string

	// CephEnabled は marmotd.json の ceph_enabled が true かどうか。trueの場合、ノードへ
	// Cephクライアント(ceph-common、カーネルモジュール前提)と/etc/ceph設定を導入する。
	CephEnabled bool
	// CephConfContent は /etc/ceph/ceph.conf の内容(marmotdホストのものをそのまま配布する)。
	CephConfContent []byte
	// CephUser は Cephキーリングのユーザー名(例: "admin"、"csi-rbd")。
	// /etc/ceph/ceph.client.<CephUser>.keyring のファイル名に使用する。
	CephUser string
	// CephKeyringContent は /etc/ceph/ceph.client.<CephUser>.keyring の内容。
	CephKeyringContent []byte

	// CloudProviderEnabled は mkeConf.CloudControllerManagerEnabled の値。trueの場合、
	// kubeletユニットに--cloud-provider=externalを付与し、CCM(mke-node-controller)による
	// ノード初期化(ProviderID/NodeAddresses)を待ち受ける状態にする(フェーズ14項目4)。
	CloudProviderEnabled bool
}

// kubernetesEngineNodeRoute は、Bridge CNI選択時にノードへ追加する、
// 他ノードのPod CIDR宛の静的経路。
type kubernetesEngineNodeRoute struct {
	CIDR string
	Via  string
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
// namespace は、ノードが接続された「ノード間通信用ネットワーク」に到達するために使用する
// ネットワーク名前空間(コントロールプレーン用に作成済みのnetnsを再利用する)。空文字の場合は
// 呼び出し元プロセスの名前空間からそのままダイヤルする。
func provisionKubernetesEngineNodeSSH(address, privateKeyPath, namespace, nodeID string, data kubernetesEngineNodeProvisionData) error {
	client, err := dialKubernetesEngineNodeSSH(address, privateKeyPath, namespace)
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

	// br_netfilterが無いと、同一ノード上のPod間通信(cni0ブリッジ経由)がiptables/conntrackを
	// 素通りし、Service ClusterIP宛通信の応答パケットがun-DNATされずクライアントに破棄される。
	if err := runner.step("enable bridge netfilter for Service ClusterIP NAT", func() error {
		return runner.run("modprobe br_netfilter && "+
			"echo br_netfilter > /etc/modules-load.d/k8s-br-netfilter.conf && "+
			"printf 'net.bridge.bridge-nf-call-iptables=1\\nnet.bridge.bridge-nf-call-ip6tables=1\\nnet.ipv4.ip_forward=1\\n' > /etc/sysctl.d/99-marmot-mke-node.conf && "+
			"sysctl --system", nil)
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
		shaURL := binURL + ".sha256"
		dest := "/usr/local/bin/" + bin
		installCmd := fmt.Sprintf(
			"tmp=$(mktemp) && "+
				"curl -fsSL %s -o \"$tmp\" && "+
				"sha=$(curl -fsSL %s | awk '{print $1}') && "+
				"echo \"$sha  $tmp\" | sha256sum -c - && "+
				"install -m 0755 \"$tmp\" %s",
			shellQuote(binURL), shellQuote(shaURL), shellQuote(dest),
		)
		if !data.ForceBinaryReplace {
			installCmd = fmt.Sprintf("test -e %s || (%s)", shellQuote(dest), installCmd)
		}
		if err := runner.step("install "+bin+" binary", func() error {
			return runner.run(installCmd, nil)
		}); err != nil {
			return err
		}
	}

	if data.NetworkKind != kubernetesEngineNetworkKindCilium {
		if err := installKubernetesEngineBridgeCNI(runner, data); err != nil {
			return err
		}
	}

	if data.CephEnabled {
		if err := installKubernetesEngineCephClient(runner, data); err != nil {
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
		{"kubelet.service", kubernetesEngineNodeKubeletUnit(data.NodeName, data.NodeIP, data.CloudProviderEnabled)},
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
		if data.ForceBinaryReplace {
			// upgrade時は、既にactiveなkubelet/kube-proxyへ新バイナリを反映させるため明示的に再起動する
			// (enable --nowは既にactiveなユニットには何もしない)。
			return runner.run("systemctl daemon-reload && systemctl enable --now containerd && systemctl restart kubelet kube-proxy", nil)
		}
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

// installKubernetesEngineBridgeCNI は、Bridge CNI（既定のnetwork.cni-plugin=none）選択時に、
// CNIプラグインバイナリの導入、ノード専用Pod CIDRを使ったbridge CNI設定の生成、
// クラスタ外向け通信のみをマスカレードするiptables設定を行う。
func installKubernetesEngineBridgeCNI(runner kubernetesEngineNodeCommandRunner, data kubernetesEngineNodeProvisionData) error {
	arch, err := detectKubernetesEngineNodeArch(runner)
	if err != nil {
		return fmt.Errorf("failed to detect node architecture: %w", err)
	}

	cniURL := fmt.Sprintf("https://github.com/containernetworking/plugins/releases/download/v%s/cni-plugins-linux-%s-v%s.tgz",
		data.CNIPluginsVersion, arch, data.CNIPluginsVersion)
	if err := runner.step("install CNI plugins", func() error {
		return runner.run(fmt.Sprintf("mkdir -p /opt/cni/bin && "+
			"(test -e /opt/cni/bin/bridge && test -e /opt/cni/bin/host-local && test -e /opt/cni/bin/loopback && test -e /opt/cni/bin/portmap) || (url=%[1]s; fn=$(basename $url); curl -fsSL $url -o /tmp/$fn && curl -fsSL $url.sha256 -o /tmp/$fn.sha256 && (cd /tmp && sha256sum -c $fn.sha256) && tar -xz -C /opt/cni/bin -f /tmp/$fn && rm -f /tmp/$fn /tmp/$fn.sha256)",
			shellQuote(cniURL)), nil)
	}); err != nil {
		return err
	}

	if err := runner.step("configure bridge CNI", func() error {
		return runner.writeFile("/etc/cni/net.d/10-bridge.conflist", "0644",
			[]byte(renderKubernetesEngineBridgeCNIConf(data.PodCIDR)))
	}); err != nil {
		return err
	}

	if err := runner.step("configure pod network NAT", func() error {
		return runner.run(kubernetesEnginePodNetworkNATScript(data.PodNetworkSupernet), nil)
	}); err != nil {
		return err
	}

	return runner.step("persist pod network NAT rules across reboot", func() error {
		return persistKubernetesEngineNodeIptablesRules(runner)
	})
}

// persistKubernetesEngineNodeIptablesRules は、iptablesルールを再起動後も維持するための
// スクリプトとsystemdユニットをノードへ配置し、現在のルールを保存する。
// この関数はべき等であり、既存クラスタのマイグレーション時にも安全に呼び出せる。
func persistKubernetesEngineNodeIptablesRules(runner kubernetesEngineNodeCommandRunner) error {
	if err := runner.run("mkdir -p /etc/marmot/iptables", nil); err != nil {
		return err
	}
	if err := runner.writeFile(kubernetesEngineNodeIptablesRestoreScriptPath, "0755",
		[]byte(kubernetesEngineNodeIptablesRestoreScript())); err != nil {
		return err
	}
	if err := runner.writeFile(kubernetesEngineNodeIptablesRestoreUnitPath, "0644",
		[]byte(kubernetesEngineNodeIptablesRestoreUnit())); err != nil {
		return err
	}
	if err := runner.run("systemctl daemon-reload && systemctl enable marmot-mke-iptables-restore.service", nil); err != nil {
		return err
	}
	return runner.run(kubernetesEngineNodeIptablesRestoreScriptPath+" save", nil)
}

// reconcileKubernetesEngineNodeIptablesSSH は、既存のBridgeノードに対してiptables永続化を
// SSH経由でベき等に適用する（marmotdアップグレード後の既存クラスタ向けマイグレーション）。
func reconcileKubernetesEngineNodeIptablesSSH(address, privateKeyPath, namespace, nodeID string) error {
	client, err := dialKubernetesEngineNodeSSH(address, privateKeyPath, namespace)
	if err != nil {
		return fmt.Errorf("failed to connect to node %s for iptables reconciliation: %w", nodeID, err)
	}
	defer func() { _ = client.Close() }()

	runner := &kubernetesEngineNodeSSHRunner{client: client, resourceID: nodeID}
	return runner.step("persist pod network NAT rules across reboot", func() error {
		return persistKubernetesEngineNodeIptablesRules(runner)
	})
}

// kubernetesEngineNodeIptablesRulesPath は、installKubernetesEngineBridgeCNIが設定した
// Podネットワークegress用NAT(マスカレード)ルールを保存するファイル。iptablesルールは
// デフォルトで永続化されないため、ノード再起動でルールが失われ、Pod発の外部向け通信
// (ping・DNS解決の上流問い合わせ等)が失敗する問題への対応。
const kubernetesEngineNodeIptablesRulesPath = "/etc/marmot/iptables/mke-node-rules.v4"
const kubernetesEngineNodeIptablesRestoreScriptPath = "/usr/local/sbin/marmot-mke-iptables-restore.sh"
const kubernetesEngineNodeIptablesRestoreUnitPath = "/etc/systemd/system/marmot-mke-iptables-restore.service"

// kubernetesEngineNodeIptablesRestoreScript は、Gatewayのiptables永続化(marmot-gateway-iptables-restore.sh)
// と同様に、引数なしなら保存済みルールを復元し、"save"引数なら現在のルールを保存するスクリプト。
func kubernetesEngineNodeIptablesRestoreScript() string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

RULES_FILE="%s"
if [[ "${1:-}" == "save" ]]; then
  iptables-save > "${RULES_FILE}"
  exit 0
fi

if [[ ! -s "${RULES_FILE}" ]]; then
  exit 0
fi

IPTABLES_RESTORE="$(command -v iptables-restore || true)"
if [[ -z "${IPTABLES_RESTORE}" ]]; then
  IPTABLES_RESTORE="/sbin/iptables-restore"
fi

"${IPTABLES_RESTORE}" < "${RULES_FILE}"
`, kubernetesEngineNodeIptablesRulesPath)
}

// kubernetesEngineNodeIptablesRestoreUnit は、起動時にルールを復元(ExecStart)し、
// 停止時に現在のルールを保存(ExecStop)するoneshotサービス。
func kubernetesEngineNodeIptablesRestoreUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Marmot KubernetesEngine node iptables restore
DefaultDependencies=no
Wants=network-pre.target
After=network-pre.target local-fs.target
Before=network.target

[Service]
Type=oneshot
ExecStart=%[1]s
ExecStop=%[1]s save
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`, kubernetesEngineNodeIptablesRestoreScriptPath)
}

// installKubernetesEngineCephClient は、Ceph連携(ceph_enabled=true)が有効な場合に、ノードへ
// Cephクライアントパッケージと/etc/ceph配下の設定(ceph.conf/keyring)を導入する。Ceph CSIの
// node-pluginがホストのカーネルモジュール(rbd/ceph)経由でボリュームをマウントするために必要。
func installKubernetesEngineCephClient(runner kubernetesEngineNodeCommandRunner, data kubernetesEngineNodeProvisionData) error {
	if err := runner.step("install Ceph client packages", func() error {
		return runner.run("DEBIAN_FRONTEND=noninteractive apt-get install -y ceph-common", nil)
	}); err != nil {
		return err
	}
	return runner.step("configure Ceph client", func() error {
		if err := runner.run("mkdir -p /etc/ceph", nil); err != nil {
			return err
		}
		if err := runner.writeFile("/etc/ceph/ceph.conf", "0644", data.CephConfContent); err != nil {
			return err
		}
		return runner.writeFile(fmt.Sprintf("/etc/ceph/ceph.client.%s.keyring", data.CephUser), "0600", data.CephKeyringContent)
	})
}

// renderKubernetesEngineBridgeCNIConf は、ノード専用のPod CIDRをhost-local IPAMで
// 払い出すbridge CNIのconflistを生成する。ノード間のPod間通信は
// reconcileKubernetesEngineNodeRoutes が設定する静的経路で疎通させるため、
// ipMasqはfalseとする（クラスタ外向けの通信は別途iptablesでマスカレードする）。
// isDefaultGateway:true がゲートウェイ経由のデフォルトルートを自動追加するため、
// ipam.routesに同じ0.0.0.0/0を重複指定しない（重複指定すると"file exists"で
// サンドボックス作成に失敗する）。
func renderKubernetesEngineBridgeCNIConf(podCIDR string) string {
	return fmt.Sprintf(`{
  "cniVersion": "1.0.0",
  "name": "mke-bridge",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "isDefaultGateway": true,
      "ipMasq": false,
      "hairpinMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{ "subnet": "%s" }]]
      }
    },
    { "type": "portmap", "capabilities": { "portMappings": true } }
  ]
}
`, podCIDR)
}

// kubernetesEnginePodNetworkNATScript は、Podネットワーク（podNetworkSupernet）宛の
// 通信はマスカレードせず直接ルーティングし、それ以外（クラスタ外）向けの通信のみを
// マスカレードするiptablesルールを冪等に追加するシェルスクリプトを返す。
func kubernetesEnginePodNetworkNATScript(podNetworkSupernet string) string {
	exempt := fmt.Sprintf("-s %s -d %s -j RETURN", podNetworkSupernet, podNetworkSupernet)
	masquerade := fmt.Sprintf("-s %s ! -d %s -j MASQUERADE", podNetworkSupernet, podNetworkSupernet)
	return fmt.Sprintf(
		"iptables -t nat -C POSTROUTING %s 2>/dev/null || iptables -t nat -I POSTROUTING %s\n"+
			"iptables -t nat -C POSTROUTING %s 2>/dev/null || iptables -t nat -A POSTROUTING %s",
		exempt, exempt, masquerade, masquerade)
}

// runKubernetesEngineNodeRouting はテストからモック可能にするための注入ポイント。
var runKubernetesEngineNodeRouting = applyKubernetesEngineNodeRoutes

// applyKubernetesEngineNodeRoutes は、指定ノードへSSH接続し、routesで示された
// 他ノードのPod CIDR宛の静的経路を(ip route replaceで冪等に)設定する。
func applyKubernetesEngineNodeRoutes(address, privateKeyPath, namespace, nodeID string, routes []kubernetesEngineNodeRoute) error {
	if len(routes) == 0 {
		return nil
	}
	client, err := dialKubernetesEngineNodeSSH(address, privateKeyPath, namespace)
	if err != nil {
		return fmt.Errorf("failed to connect to node for routing: %w", err)
	}
	defer func() { _ = client.Close() }()

	runner := &kubernetesEngineNodeSSHRunner{client: client, resourceID: nodeID}
	return runner.step("configure pod network routes", func() error {
		cmds := make([]string, 0, len(routes))
		for _, route := range routes {
			cmds = append(cmds, fmt.Sprintf("ip route replace %s via %s", shellQuote(route.CIDR), shellQuote(route.Via)))
		}
		return runner.run(strings.Join(cmds, " && "), nil)
	})
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

func kubernetesEngineNodeKubeletUnit(nodeName, nodeIP string, cloudProviderEnabled bool) string {
	cloudProviderArg := ""
	if cloudProviderEnabled {
		// CCM(mke-node-controller)がProviderID/NodeAddressesを設定するまで、kubeletは
		// node.cloudprovider.kubernetes.io/uninitialized taintを付与した状態でNodeを登録する。
		cloudProviderArg = " --cloud-provider=external"
	}
	return fmt.Sprintf(`[Unit]
Description=Kubernetes Kubelet
Wants=network-online.target containerd.service
After=network-online.target containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet --config=/etc/kubernetes/kubelet/config.yaml --kubeconfig=/etc/kubernetes/kubelet.kubeconfig --hostname-override=%s --node-ip=%s%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, nodeName, nodeIP, cloudProviderArg)
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

// kubernetesEngineNodeDialTCP は、必要に応じて指定されたネットワーク名前空間からTCP接続を確立する。
// テストからモック可能にするための注入ポイント。
var kubernetesEngineNodeDialTCP = func(namespace, network, address string) (net.Conn, error) {
	return DialInKubernetesEngineNetworkNamespace(namespace, network, address, kubernetesEngineNodeSSHDialTimeout)
}

func dialKubernetesEngineNodeSSH(address, privateKeyPath, namespace string) (*ssh.Client, error) {
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
	targetAddr := net.JoinHostPort(trimmed, fmt.Sprintf("%d", kubernetesEngineNodeSSHPort))

	conn, err := kubernetesEngineNodeDialTCP(namespace, "tcp", targetAddr)
	if err != nil {
		return nil, err
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

// kubernetesEngineNodeSSHRunner executes commands on a KubernetesEngine node over SSH,
// streaming each output line to slog so long-running steps stay visible in CI logs.
type kubernetesEngineNodeSSHRunner struct {
	client     *ssh.Client
	resourceID string
}

func (r *kubernetesEngineNodeSSHRunner) step(name string, fn func() error) error {
	slog.Debug("kubernetes-engine-node provision step", "id", r.resourceID, "step", name)
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
		slog.Debug("kubernetes-engine-node provision progress", "id", l.resourceID, "line", line)
	}
	return len(p), nil
}
