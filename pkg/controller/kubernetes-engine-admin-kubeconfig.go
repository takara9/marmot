package controller

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// kubernetesEngineAdminUserName は kubectl/mactl からの外部アクセス用管理者証明書の
// CommonName/ユーザー名。system:masters に属し、フェーズ10のkubectlアクセス経路で使用する。
const kubernetesEngineAdminUserName = "kubernetes-admin"

type adminKubeconfig struct {
	APIVersion     string              `yaml:"apiVersion"`
	Kind           string              `yaml:"kind"`
	CurrentContext string              `yaml:"current-context"`
	Clusters       []adminNamedCluster `yaml:"clusters"`
	Contexts       []adminNamedContext `yaml:"contexts"`
	Users          []adminNamedUser    `yaml:"users"`
}

type adminNamedCluster struct {
	Name    string             `yaml:"name"`
	Cluster adminClusterConfig `yaml:"cluster"`
}

type adminClusterConfig struct {
	CertificateAuthorityData string `yaml:"certificate-authority-data"`
	Server                   string `yaml:"server"`
}

type adminNamedContext struct {
	Name    string             `yaml:"name"`
	Context adminContextConfig `yaml:"context"`
}

type adminContextConfig struct {
	Cluster string `yaml:"cluster"`
	User    string `yaml:"user"`
}

type adminNamedUser struct {
	Name string          `yaml:"name"`
	User adminUserConfig `yaml:"user"`
}

type adminUserConfig struct {
	ClientCertificateData string `yaml:"client-certificate-data"`
	ClientKeyData         string `yaml:"client-key-data"`
}

// EnsureKubernetesEngineAdminKubeconfig は system:masters 権限を持つ管理者証明書を
// 冪等に発行し、hostBindAddress:apiServerPort を server に指定したkubeconfigを組み立てて返す。
// コントロールプレーンプロセス用kubeconfig(writeControlPlaneKubeconfig)はマルモtdホスト上の
// ファイルパス参照を使うが、こちらはダウンロード先のマシンにそのパスが存在しない前提のため、
// CA/クライアント証明書・鍵をbase64データとして埋め込む。
func EnsureKubernetesEngineAdminKubeconfig(pkiDir, clusterName, hostBindAddress string, apiServerPort int) ([]byte, error) {
	name, err := validateKubernetesEnginePkiClusterName(clusterName)
	if err != nil {
		return nil, err
	}
	clusterName = name
	hostBindAddress = strings.TrimSpace(hostBindAddress)
	if net.ParseIP(hostBindAddress) == nil {
		return nil, fmt.Errorf("invalid control plane host bind address %q", hostBindAddress)
	}
	if apiServerPort <= 0 {
		return nil, fmt.Errorf("invalid API server port %d", apiServerPort)
	}

	caCertPath, _, err := EnsureKubernetesEngineCA(pkiDir, clusterName)
	if err != nil {
		return nil, err
	}
	certPath, keyPath, err := IssueKubernetesEngineCertificate(pkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          kubernetesEngineAdminUserName,
		CommonName:    kubernetesEngineAdminUserName,
		Organizations: []string{"system:masters"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return nil, err
	}

	caData, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	const kubeconfigClusterName = "mke"
	config := adminKubeconfig{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: kubernetesEngineAdminUserName + "@" + kubeconfigClusterName,
		Clusters: []adminNamedCluster{{Name: kubeconfigClusterName, Cluster: adminClusterConfig{
			CertificateAuthorityData: base64.StdEncoding.EncodeToString(caData),
			Server:                   fmt.Sprintf("https://%s:%d", hostBindAddress, apiServerPort),
		}}},
		Contexts: []adminNamedContext{{Name: kubernetesEngineAdminUserName + "@" + kubeconfigClusterName, Context: adminContextConfig{
			Cluster: kubeconfigClusterName,
			User:    kubernetesEngineAdminUserName,
		}}},
		Users: []adminNamedUser{{Name: kubernetesEngineAdminUserName, User: adminUserConfig{
			ClientCertificateData: base64.StdEncoding.EncodeToString(certData),
			ClientKeyData:         base64.StdEncoding.EncodeToString(keyData),
		}}},
	}
	return yaml.Marshal(config)
}
