package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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
)

var (
	kubernetesEngineNodePrivateKeyPath  = marmotd.GatewayPrivateKeyPath()
	kubernetesEngineNodePublicKeyPath   = marmotd.GatewayPublicKeyPath()
	ensureKubernetesEngineNodeSSHAssets = marmotd.EnsureGatewayRuntimeAssets
	runKubernetesEngineNodeProvision    = provisionKubernetesEngineNodeSSH
	queryKubernetesEngineNodes          = queryKubernetesEngineNodesCommand
)

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
	contents := make([][]byte, len(paths))
	for index, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		contents[index] = content
	}
	endpoint := fmt.Sprintf("https://%s:%d", *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)
	data := kubernetesEngineNodeProvisionData{
		NodeName:            nodeName,
		NodeIP:              nodeIP,
		KubernetesVersion:   strings.TrimSpace(*ke.Status.ResolvedKubernetesVersion),
		ContainerdVersion:   strings.TrimPrefix(strings.TrimSpace(mkeConf.ContainerdVersion), "v"),
		RuncVersion:         strings.TrimPrefix(strings.TrimSpace(mkeConf.RuncVersion), "v"),
		CACert:              contents[0],
		KubeletCert:         contents[1],
		KubeletKey:          contents[2],
		KubeProxyCert:       contents[3],
		KubeProxyKey:        contents[4],
		KubeletKubeconfig:   []byte(renderKubernetesEngineNodeKubeconfig(endpoint, "system:node", "/etc/kubernetes/pki/kubelet.crt", "/etc/kubernetes/pki/kubelet.key")),
		KubeProxyKubeconfig: []byte(renderKubernetesEngineNodeKubeconfig(endpoint, "system:kube-proxy", "/etc/kubernetes/pki/kube-proxy.crt", "/etc/kubernetes/pki/kube-proxy.key")),
		KubeletConfig:       []byte(renderKubernetesEngineKubeletConfig()),
		KubeProxyConfig:     []byte(renderKubernetesEngineKubeProxyConfig()),
	}
	nodeID := fmt.Sprintf("%s-%s", api.KubernetesEngineID(ke), nodeName)
	return runKubernetesEngineNodeProvision(nodeIP, kubernetesEngineNodePrivateKeyPath, nodeID, data)
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
