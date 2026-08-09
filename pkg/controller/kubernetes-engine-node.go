package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	kubernetesEngineNodeLabelOwner    = "kubernetesEngineId"
	kubernetesEngineNodeLabelRole     = "kubernetesEngineRole"
	kubernetesEngineNodeRoleValue     = "node"
	kubernetesEngineNodeLabelIndex    = "kubernetesEngineNodeIndex"
	defaultKubernetesEngineNodeCPU    = 2
	defaultKubernetesEngineNodeMemory = 2048
	kubernetesEngineNodePlaybookDir   = "/var/lib/marmot/ansible-playbooks"
)

var (
	kubernetesEngineNodePrivateKeyPath  = marmotd.GatewayPrivateKeyPath()
	kubernetesEngineNodePublicKeyPath   = marmotd.GatewayPublicKeyPath()
	ensureKubernetesEngineNodeSSHAssets = marmotd.EnsureGatewayRuntimeAssets
	runKubernetesEngineNodePlaybook     = runKubernetesEngineNodePlaybookCommand
	queryKubernetesEngineNodes          = queryKubernetesEngineNodesCommand
)

type kubernetesEngineNodePlaybookData struct {
	NodeName            string
	NodeIP              string
	APIServerEndpoint   string
	KubernetesVersion   string
	ContainerdVersion   string
	RuncVersion         string
	CACertBase64        string
	KubeletCertBase64   string
	KubeletKeyBase64    string
	KubeProxyCertBase64 string
	KubeProxyKeyBase64  string
}

type kubernetesNodeList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

const kubernetesEngineNodePlaybookTemplate = `---
- name: Configure Marmot Kubernetes node
  hosts: all
  become: true
  gather_facts: true
  vars:
    kubernetes_version: {{ printf "%q" .KubernetesVersion }}
    containerd_version: {{ printf "%q" .ContainerdVersion }}
    runc_version: {{ printf "%q" .RuncVersion }}
  tasks:
    - name: Install runtime dependencies
      ansible.builtin.apt:
        name:
          - ca-certificates
          - conntrack
          - curl
          - iptables
          - socat
        state: present
        update_cache: true

    - name: Select release architecture
      ansible.builtin.set_fact:
        release_arch: {{ "\"{{ 'amd64' if ansible_architecture == 'x86_64' else 'arm64' }}\"" }}

    - name: Create Kubernetes directories
      ansible.builtin.file:
        path: {{ "{{ item }}" }}
        state: directory
        owner: root
        group: root
        mode: "0755"
      loop:
        - /etc/kubernetes
        - /etc/kubernetes/pki
        - /etc/kubernetes/kubelet
        - /etc/kubernetes/kube-proxy
        - /opt/cni/bin

    - name: Install containerd
      ansible.builtin.unarchive:
        src: {{ "https://github.com/containerd/containerd/releases/download/v{{ containerd_version }}/containerd-{{ containerd_version }}-linux-{{ release_arch }}.tar.gz" }}
        dest: /usr/local
        remote_src: true
        creates: /usr/local/bin/containerd

    - name: Install runc
      ansible.builtin.get_url:
        url: {{ "https://github.com/opencontainers/runc/releases/download/v{{ runc_version }}/runc.{{ release_arch }}" }}
        dest: /usr/local/sbin/runc
        mode: "0755"

    - name: Install Kubernetes node binaries
      ansible.builtin.get_url:
        url: {{ "https://dl.k8s.io/release/{{ kubernetes_version }}/bin/linux/{{ release_arch }}/{{ item }}" }}
        dest: {{ "\"{{ '/usr/local/bin/' + item }}\"" }}
        mode: "0755"
      loop:
        - kubelet
        - kube-proxy

    - name: Install cluster credentials and configuration
      ansible.builtin.copy:
        content: {{ "{{ item.content | b64decode }}" }}
        dest: {{ "{{ item.path }}" }}
        owner: root
        group: root
        mode: "0600"
      no_log: true
      loop:
        - path: /etc/kubernetes/pki/ca.crt
          content: {{ printf "%q" .CACertBase64 }}
        - path: /etc/kubernetes/pki/kubelet.crt
          content: {{ printf "%q" .KubeletCertBase64 }}
        - path: /etc/kubernetes/pki/kubelet.key
          content: {{ printf "%q" .KubeletKeyBase64 }}
        - path: /etc/kubernetes/pki/kube-proxy.crt
          content: {{ printf "%q" .KubeProxyCertBase64 }}
        - path: /etc/kubernetes/pki/kube-proxy.key
          content: {{ printf "%q" .KubeProxyKeyBase64 }}
        - path: /etc/kubernetes/kubelet.kubeconfig
          content: {{ printf "%q" (kubeconfigBase64 .APIServerEndpoint "system:node" "/etc/kubernetes/pki/kubelet.crt" "/etc/kubernetes/pki/kubelet.key") }}
        - path: /etc/kubernetes/kube-proxy.kubeconfig
          content: {{ printf "%q" (kubeconfigBase64 .APIServerEndpoint "system:kube-proxy" "/etc/kubernetes/pki/kube-proxy.crt" "/etc/kubernetes/pki/kube-proxy.key") }}
        - path: /etc/kubernetes/kubelet/config.yaml
          content: {{ printf "%q" (kubeletConfigBase64) }}
        - path: /etc/kubernetes/kube-proxy/config.yaml
          content: {{ printf "%q" (kubeProxyConfigBase64) }}

    - name: Create containerd configuration directory
      ansible.builtin.file:
        path: /etc/containerd
        state: directory
        mode: "0755"

    - name: Generate containerd configuration
      ansible.builtin.shell: |
        set -eu
        /usr/local/bin/containerd config default > /etc/containerd/config.toml
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
      args:
        creates: /etc/containerd/config.toml

    - name: Install containerd systemd unit
      ansible.builtin.copy:
        dest: /etc/systemd/system/containerd.service
        mode: "0644"
        content: |
          [Unit]
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

    - name: Install kubelet systemd unit
      ansible.builtin.copy:
        dest: /etc/systemd/system/kubelet.service
        mode: "0644"
        content: |
          [Unit]
          Description=Kubernetes Kubelet
          Wants=network-online.target containerd.service
          After=network-online.target containerd.service

          [Service]
          ExecStart=/usr/local/bin/kubelet --config=/etc/kubernetes/kubelet/config.yaml --kubeconfig=/etc/kubernetes/kubelet.kubeconfig --hostname-override={{ .NodeName }} --node-ip={{ .NodeIP }}
          Restart=always
          RestartSec=5

          [Install]
          WantedBy=multi-user.target

    - name: Install kube-proxy systemd unit
      ansible.builtin.copy:
        dest: /etc/systemd/system/kube-proxy.service
        mode: "0644"
        content: |
          [Unit]
          Description=Kubernetes Kube Proxy
          Wants=network-online.target
          After=network-online.target

          [Service]
          ExecStart=/usr/local/bin/kube-proxy --config=/etc/kubernetes/kube-proxy/config.yaml
          Restart=always
          RestartSec=5

          [Install]
          WantedBy=multi-user.target

    - name: Enable Kubernetes node services
      ansible.builtin.systemd:
        name: {{ "{{ item }}" }}
        enabled: true
        state: started
        daemon_reload: true
      loop:
        - containerd
        - kubelet
        - kube-proxy
`

func ProvisionKubernetesEngineNodes(database *db.Database, mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) (bool, error) {
	if ke.Spec.Nodes < 1 {
		return false, fmt.Errorf("spec.nodes must be greater than zero")
	}
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil || ke.Status.ResolvedKubernetesVersion == nil {
		return false, fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	if err := ensureKubernetesEngineNodeSSHAssets(); err != nil {
		return false, fmt.Errorf("failed to prepare KubernetesEngine node SSH assets: %w", err)
	}

	servers, err := findKubernetesEngineNodeServers(database, ke)
	if err != nil {
		return false, err
	}
	if len(servers) > ke.Spec.Nodes {
		return false, fmt.Errorf("KubernetesEngine has %d nodes, expected %d; scale-in is handled in phase 12", len(servers), ke.Spec.Nodes)
	}
	if len(servers) < ke.Spec.Nodes {
		if err := createMissingKubernetesEngineNodeServers(database, ke, servers); err != nil {
			return false, err
		}
		return false, nil
	}

	expectedNames := make([]string, 0, len(servers))
	for _, server := range servers {
		if server.Status == nil {
			return false, nil
		}
		switch server.Status.StatusCode {
		case db.SERVER_RUNNING:
		case db.SERVER_ERROR:
			message := "node server entered error state"
			if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
				message = strings.TrimSpace(*server.Status.Message)
			}
			return false, fmt.Errorf("node %s: %s", server.Metadata.Name, message)
		default:
			return false, nil
		}
		nodeIP, err := kubernetesEngineNodeInternalIP(server, kubernetesEngineNetworkName(ke))
		if err != nil {
			return false, err
		}
		if err := configureKubernetesEngineNode(mkeConf, ke, server.Metadata.Name, nodeIP); err != nil {
			return false, err
		}
		expectedNames = append(expectedNames, server.Metadata.Name)
	}

	ready, err := queryKubernetesEngineNodes(ke, expectedNames)
	if err != nil {
		return false, err
	}
	return ready, nil
}

func findKubernetesEngineNodeServers(database *db.Database, ke api.KubernetesEngine) ([]api.Server, error) {
	servers, err := database.GetServers()
	if err != nil {
		return nil, err
	}
	owner := api.KubernetesEngineID(ke)
	owned := make([]api.Server, 0, ke.Spec.Nodes)
	for _, server := range servers {
		if server.Metadata.Labels == nil {
			continue
		}
		labels := *server.Metadata.Labels
		if labels[kubernetesEngineNodeLabelOwner] == owner && labels[kubernetesEngineNodeLabelRole] == kubernetesEngineNodeRoleValue {
			owned = append(owned, server)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Metadata.Name < owned[j].Metadata.Name })
	return owned, nil
}

func createMissingKubernetesEngineNodeServers(database *db.Database, ke api.KubernetesEngine, existing []api.Server) error {
	existingNames := make(map[string]bool, len(existing))
	for _, server := range existing {
		existingNames[server.Metadata.Name] = true
	}
	allServers, err := database.GetServers()
	if err != nil {
		return err
	}
	serverOwnersByName := make(map[string]string, len(allServers))
	for _, server := range allServers {
		owner := ""
		if server.Metadata.Labels != nil {
			owner, _ = (*server.Metadata.Labels)[kubernetesEngineNodeLabelOwner].(string)
		}
		serverOwnersByName[server.Metadata.Name] = owner
	}
	publicKey, err := os.ReadFile(kubernetesEngineNodePublicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read KubernetesEngine node public key: %w", err)
	}
	for index := 0; index < ke.Spec.Nodes; index++ {
		name := kubernetesEngineNodeName(ke, index)
		if existingNames[name] {
			continue
		}
		if owner, found := serverOwnersByName[name]; found {
			return fmt.Errorf("server name %s is already used by owner %q", name, owner)
		}
		spec, err := buildKubernetesEngineNodeServerSpec(ke, index, strings.TrimSpace(string(publicKey)))
		if err != nil {
			return err
		}
		if _, err := database.MakeServerEntry(spec); err != nil {
			return fmt.Errorf("failed to create KubernetesEngine node %s: %w", name, err)
		}
	}
	return nil
}

func buildKubernetesEngineNodeServerSpec(ke api.KubernetesEngine, index int, publicKey string) (api.Server, error) {
	externalNetwork := "default"
	cpu := defaultKubernetesEngineNodeCPU
	memory := defaultKubernetesEngineNodeMemory
	if ke.Spec.NodeSpec != nil {
		if ke.Spec.NodeSpec.Cpu != nil {
			cpu = *ke.Spec.NodeSpec.Cpu
		}
		if ke.Spec.NodeSpec.Memory != nil {
			memory = *ke.Spec.NodeSpec.Memory
		}
		if ke.Spec.NodeSpec.Network != nil && ke.Spec.NodeSpec.Network.External != nil {
			externalNetwork = strings.TrimSpace(*ke.Spec.NodeSpec.Network.External)
		}
	}
	if cpu < 1 || memory < 1 {
		return api.Server{}, fmt.Errorf("nodeSpec cpu and memory must be greater than zero")
	}
	if externalNetwork != "default" && externalNetwork != "host-bridge" {
		return api.Server{}, fmt.Errorf("nodeSpec.network.external must be default or host-bridge")
	}
	if strings.TrimSpace(publicKey) == "" {
		return api.Server{}, fmt.Errorf("KubernetesEngine node public key is empty")
	}

	name := kubernetesEngineNodeName(ke, index)
	labels := map[string]interface{}{
		kubernetesEngineNodeLabelOwner: api.KubernetesEngineID(ke),
		kubernetesEngineNodeLabelRole:  kubernetesEngineNodeRoleValue,
		kubernetesEngineNodeLabelIndex: index,
	}
	metadata := api.Metadata{Name: name, Labels: &labels}
	if ke.Metadata.NodeName != nil && strings.TrimSpace(*ke.Metadata.NodeName) != "" {
		metadata.NodeName = util.StringPtr(strings.TrimSpace(*ke.Metadata.NodeName))
	}
	nics := []api.NetworkInterface{
		{Networkname: externalNetwork},
		{Networkname: kubernetesEngineNetworkName(ke)},
	}
	return api.Server{
		ApiVersion: "v1",
		Kind:       "Server",
		Metadata:   metadata,
		Spec: api.ServerSpec{
			Cpu:              util.IntPtrInt(cpu),
			Memory:           util.IntPtrInt(memory),
			OsVariant:        util.StringPtr("ubuntu24.04"),
			NetworkInterface: &nics,
			Auth:             &api.Auth{PublicKey: util.StringPtr(publicKey), User: util.StringPtr("root")},
		},
	}, nil
}

func kubernetesEngineNodeName(ke api.KubernetesEngine, index int) string {
	return fmt.Sprintf("mke-%s-node-%d", strings.TrimSpace(ke.Metadata.Name), index+1)
}

func kubernetesEngineNodeInternalIP(server api.Server, networkName string) (string, error) {
	if server.Spec.NetworkInterface == nil {
		return "", fmt.Errorf("node %s has no network interfaces", server.Metadata.Name)
	}
	for _, nic := range *server.Spec.NetworkInterface {
		if nic.Networkname == networkName && nic.Address != nil && strings.TrimSpace(*nic.Address) != "" {
			return strings.TrimSpace(*nic.Address), nil
		}
	}
	return "", fmt.Errorf("node %s has no address on network %s", server.Metadata.Name, networkName)
}

func configureKubernetesEngineNode(mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine, nodeName, nodeIP string) error {
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	caPath, _ := KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, clusterName)
	kubeletCertPath, kubeletKeyPath, err := IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "kubelet-" + nodeName,
		CommonName:    "system:node:" + nodeName,
		Organizations: []string{"system:nodes"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return err
	}
	kubeProxyCertPath, kubeProxyKeyPath, err := IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:       "kube-proxy-" + nodeName,
		CommonName: "system:kube-proxy",
		Usage:      KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return err
	}
	paths := []string{caPath, kubeletCertPath, kubeletKeyPath, kubeProxyCertPath, kubeProxyKeyPath}
	encoded := make([]string, len(paths))
	for index, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		encoded[index] = base64.StdEncoding.EncodeToString(data)
	}
	endpoint := fmt.Sprintf("https://%s:%d", *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)
	playbookPath := filepath.Join(kubernetesEngineNodePlaybookDir, fmt.Sprintf("mke-node-%s-%s.yaml", api.KubernetesEngineID(ke), nodeName))
	data := kubernetesEngineNodePlaybookData{
		NodeName:            nodeName,
		NodeIP:              nodeIP,
		APIServerEndpoint:   endpoint,
		KubernetesVersion:   strings.TrimSpace(*ke.Status.ResolvedKubernetesVersion),
		ContainerdVersion:   strings.TrimPrefix(strings.TrimSpace(mkeConf.ContainerdVersion), "v"),
		RuncVersion:         strings.TrimPrefix(strings.TrimSpace(mkeConf.RuncVersion), "v"),
		CACertBase64:        encoded[0],
		KubeletCertBase64:   encoded[1],
		KubeletKeyBase64:    encoded[2],
		KubeProxyCertBase64: encoded[3],
		KubeProxyKeyBase64:  encoded[4],
	}
	if err := renderKubernetesEngineNodePlaybook(playbookPath, data); err != nil {
		return err
	}
	return runKubernetesEngineNodePlaybook(playbookPath, nodeIP, kubernetesEngineNodePrivateKeyPath)
}

func renderKubernetesEngineNodePlaybook(path string, data kubernetesEngineNodePlaybookData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	functions := template.FuncMap{
		"kubeconfigBase64": func(endpoint, user, certPath, keyPath string) string {
			return base64.StdEncoding.EncodeToString([]byte(renderKubernetesEngineNodeKubeconfig(endpoint, user, certPath, keyPath)))
		},
		"kubeletConfigBase64": func() string {
			return base64.StdEncoding.EncodeToString([]byte(renderKubernetesEngineKubeletConfig()))
		},
		"kubeProxyConfigBase64": func() string {
			return base64.StdEncoding.EncodeToString([]byte(renderKubernetesEngineKubeProxyConfig()))
		},
	}
	tmpl, err := template.New("kubernetes-engine-node").Funcs(functions).Parse(kubernetesEngineNodePlaybookTemplate)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return tmpl.Execute(file, data)
}

func renderKubernetesEngineNodeKubeconfig(endpoint, user, certPath, keyPath string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
  - name: mke
    cluster:
      server: %s
      certificate-authority: /etc/kubernetes/pki/ca.crt
users:
  - name: %s
    user:
      client-certificate: %s
      client-key: %s
contexts:
  - name: %s@mke
    context:
      cluster: mke
      user: %s
current-context: %s@mke
`, endpoint, user, certPath, keyPath, user, user, user)
}

func renderKubernetesEngineKubeletConfig() string {
	return `apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
authentication:
  x509:
    clientCAFile: /etc/kubernetes/pki/ca.crt
authorization:
  mode: Webhook
cgroupDriver: systemd
clusterDNS:
  - 10.96.0.10
clusterDomain: cluster.local
containerRuntimeEndpoint: unix:///run/containerd/containerd.sock
failSwapOn: false
`
}

func renderKubernetesEngineKubeProxyConfig() string {
	return `apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/kube-proxy.kubeconfig
mode: iptables
`
}

func runKubernetesEngineNodePlaybookCommand(playbookPath, address, privateKeyPath string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("node address is empty")
	}
	if _, err := os.Stat(privateKeyPath); err != nil {
		return fmt.Errorf("private key is not available: %w", err)
	}
	args := []string{
		"-i", strings.TrimSpace(address) + ",",
		playbookPath,
		"--private-key", privateKeyPath,
		"-u", "root",
		"--ssh-common-args", "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
	}
	return runAnsiblePlaybookWithLogging(args, "kubernetes-engine-node", filepath.Base(playbookPath))
}

func queryKubernetesEngineNodesCommand(ke api.KubernetesEngine, expectedNames []string) (bool, error) {
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	adminCertPath, adminKeyPath, err := IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "controller-admin",
		CommonName:    "mke-controller-admin",
		Organizations: []string{"system:masters"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return false, err
	}
	caPath, _ := KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, clusterName)
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return false, err
	}
	endpoint := fmt.Sprintf("https://%s:%d/api/v1/nodes", *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ip", "netns", "exec", namespace, "curl", "--fail", "--silent", "--show-error", "--cacert", caPath, "--cert", adminCertPath, "--key", adminKeyPath, endpoint).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to list Kubernetes nodes: %w (output=%s)", err, strings.TrimSpace(string(output)))
	}
	return kubernetesEngineNodesReady(output, expectedNames)
}

func kubernetesEngineNodesReady(data []byte, expectedNames []string) (bool, error) {
	var nodeList kubernetesNodeList
	if err := json.Unmarshal(data, &nodeList); err != nil {
		return false, fmt.Errorf("failed to decode Kubernetes node list: %w", err)
	}
	ready := make(map[string]bool, len(nodeList.Items))
	for _, item := range nodeList.Items {
		for _, condition := range item.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				ready[item.Metadata.Name] = true
			}
		}
	}
	for _, name := range expectedNames {
		if !ready[name] {
			return false, nil
		}
	}
	return true, nil
}
