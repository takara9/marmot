package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// kubeconfigFile は kubectl 標準のkubeconfig形式のうち、mke-node-controllerが
// 必要とする最小限のフィールドのみを表す。mke-node-controllerはmarmotdホスト上で動作し、
// EnsureKubernetesEngineCloudControllerManagerKubeconfigが生成するkubeconfigはファイルパス
// 参照形式(certificate-authority/client-certificate/client-key)を使うため、base64埋め込み形式
// (*-data)に加えてパス参照形式にも対応する。
type kubeconfigFile struct {
	Clusters []struct {
		Cluster struct {
			CertificateAuthority     string `yaml:"certificate-authority"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			Server                   string `yaml:"server"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		User struct {
			ClientCertificate     string `yaml:"client-certificate"`
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKey             string `yaml:"client-key"`
			ClientKeyData         string `yaml:"client-key-data"`
		} `yaml:"user"`
	} `yaml:"users"`
}

// loadKubeconfigBytes は、dataフィールド(base64埋め込み)が設定されていればそれをデコードし、
// 空であればpathフィールド(ファイルパス参照)からそのまま読み込む。
func loadKubeconfigBytes(data, path string) ([]byte, error) {
	if data != "" {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %w", err)
		}
		return decoded, nil
	}
	if path == "" {
		return nil, fmt.Errorf("neither embedded data nor file path is set")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", path, err)
	}
	return content, nil
}

// kubeClient は client-go 等の外部依存を追加せず、kubeconfigのクライアント証明書を
// 使ったTLS HTTPアクセスのみでkube-apiserverの最小限のREST APIを呼び出す。
type kubeClient struct {
	httpClient *http.Client
	server     string
}

// loadKubeClient は kubeconfigファイルを読み込み、TLSクライアント証明書を設定した
// kubeClientを組み立てる。
func loadKubeClient(path string) (*kubeClient, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubeconfig: %w", err)
	}
	var cfg kubeconfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}
	if len(cfg.Clusters) == 0 || len(cfg.Users) == 0 {
		return nil, fmt.Errorf("kubeconfig is missing clusters or users")
	}
	cluster := cfg.Clusters[0].Cluster
	user := cfg.Users[0].User

	caData, err := loadKubeconfigBytes(cluster.CertificateAuthorityData, cluster.CertificateAuthority)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate-authority: %w", err)
	}
	certData, err := loadKubeconfigBytes(user.ClientCertificateData, user.ClientCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to load client-certificate: %w", err)
	}
	keyData, err := loadKubeconfigBytes(user.ClientKeyData, user.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client-key: %w", err)
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("failed to parse certificate authority data")
	}

	server := strings.TrimRight(strings.TrimSpace(cluster.Server), "/")
	if server == "" {
		return nil, fmt.Errorf("kubeconfig cluster server is empty")
	}

	return &kubeClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      pool,
					Certificates: []tls.Certificate{cert},
				},
			},
		},
		server: server,
	}, nil
}

// nodeTaint は Node.spec.taints[] の必要最小限の表現。
type nodeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

// nodeInfo は kube-apiserverから取得するNodeの必要最小限の情報。
type nodeInfo struct {
	Name       string
	InternalIP string
	ExternalIP string
	ProviderID string
	Taints     []nodeTaint
}

type nodeList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			ProviderID string      `json:"providerID"`
			Taints     []nodeTaint `json:"taints"`
		} `json:"spec"`
		Status struct {
			Addresses []struct {
				Type    string `json:"type"`
				Address string `json:"address"`
			} `json:"addresses"`
		} `json:"status"`
	} `json:"items"`
}

// ListNodes はkube-apiserverから全Nodeの一覧を取得する。
func (c *kubeClient) ListNodes(ctx context.Context) ([]nodeInfo, error) {
	var list nodeList
	if err := c.get(ctx, "/api/v1/nodes", &list); err != nil {
		return nil, err
	}
	nodes := make([]nodeInfo, 0, len(list.Items))
	for _, item := range list.Items {
		node := nodeInfo{
			Name:       item.Metadata.Name,
			ProviderID: item.Spec.ProviderID,
			Taints:     item.Spec.Taints,
		}
		for _, addr := range item.Status.Addresses {
			switch addr.Type {
			case "InternalIP":
				node.InternalIP = addr.Address
			case "ExternalIP":
				node.ExternalIP = addr.Address
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (c *kubeClient) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.server+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *kubeClient) patch(ctx context.Context, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.server+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/merge-patch+json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, path)
	}
	return nil
}

type nodeAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// nodeStatusPatch は type=merge-patch+json で送信する Node.status の部分更新。
// merge-patch+jsonはlist(addresses)を丸ごと置き換えるため、両方のアドレスを常に含めて送信する。
type nodeStatusPatch struct {
	Status struct {
		Addresses []nodeAddress `json:"addresses"`
	} `json:"status"`
}

// SetNodeAddresses は、Node.status.addresses に InternalIP と ExternalIP(host-bridgeアドレス)を
// セットする(kubectl get node -o wide の EXTERNAL-IP に反映される)。
func (c *kubeClient) SetNodeAddresses(ctx context.Context, name, internalIP, externalIP string) error {
	var patch nodeStatusPatch
	patch.Status.Addresses = append(patch.Status.Addresses, nodeAddress{Type: "Hostname", Address: name})
	if internalIP != "" {
		patch.Status.Addresses = append(patch.Status.Addresses, nodeAddress{Type: "InternalIP", Address: internalIP})
	}
	if externalIP != "" {
		patch.Status.Addresses = append(patch.Status.Addresses, nodeAddress{Type: "ExternalIP", Address: externalIP})
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	return c.patch(ctx, fmt.Sprintf("/api/v1/nodes/%s/status", url.PathEscape(name)), body)
}

// nodeSpecPatch は type=merge-patch+json で送信する Node.spec の部分更新。
type nodeSpecPatch struct {
	Spec struct {
		ProviderID string      `json:"providerID,omitempty"`
		Taints     []nodeTaint `json:"taints"`
	} `json:"spec"`
}

// PatchNodeSpec は、Node.spec.providerID と Node.spec.taints をセットする。
// taintsは呼び出し側が残したい全taintを含めて渡す必要がある(merge-patch+jsonは配列を丸ごと置換するため)。
func (c *kubeClient) PatchNodeSpec(ctx context.Context, name, providerID string, taints []nodeTaint) error {
	var patch nodeSpecPatch
	patch.Spec.ProviderID = providerID
	patch.Spec.Taints = taints
	if patch.Spec.Taints == nil {
		patch.Spec.Taints = []nodeTaint{}
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	return c.patch(ctx, fmt.Sprintf("/api/v1/nodes/%s", url.PathEscape(name)), body)
}

// loadBalancerServiceInfo は kube-apiserverから取得する Service(type=LoadBalancer) の
// 必要最小限の情報。
type loadBalancerServiceInfo struct {
	Namespace string
	Name      string
	VIP       string
}

type serviceList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Type string `json:"type"`
		} `json:"spec"`
		Status struct {
			LoadBalancer struct {
				Ingress []struct {
					IP string `json:"ip"`
				} `json:"ingress"`
			} `json:"loadBalancer"`
		} `json:"status"`
	} `json:"items"`
}

// ListLoadBalancerServices はkube-apiserverから全ネームスペースのServiceを取得し、
// type=LoadBalancerのものだけを返す。
func (c *kubeClient) ListLoadBalancerServices(ctx context.Context) ([]loadBalancerServiceInfo, error) {
	var list serviceList
	if err := c.get(ctx, "/api/v1/services", &list); err != nil {
		return nil, err
	}
	services := make([]loadBalancerServiceInfo, 0)
	for _, item := range list.Items {
		if item.Spec.Type != "LoadBalancer" {
			continue
		}
		vip := ""
		if len(item.Status.LoadBalancer.Ingress) > 0 {
			vip = item.Status.LoadBalancer.Ingress[0].IP
		}
		services = append(services, loadBalancerServiceInfo{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
			VIP:       vip,
		})
	}
	return services, nil
}

// serviceStatusPatch は type=merge-patch+json で送信する Service.status の部分更新。
type serviceStatusPatch struct {
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP string `json:"ip"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

// SetServiceLoadBalancerIngressIP は、Service.status.loadBalancer.ingress[0].ip に
// 払い出し済みVIPをセットする(kubectl get svc の EXTERNAL-IP に反映される)。
func (c *kubeClient) SetServiceLoadBalancerIngressIP(ctx context.Context, namespace, name, ip string) error {
	var patch serviceStatusPatch
	patch.Status.LoadBalancer.Ingress = []struct {
		IP string `json:"ip"`
	}{{IP: ip}}
	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	return c.patch(ctx, fmt.Sprintf("/api/v1/namespaces/%s/services/%s/status", url.PathEscape(namespace), url.PathEscape(name)), body)
}
