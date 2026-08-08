package controller

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultKubernetesControlPlaneConfigDir = "/var/lib/marmot/mke/control-plane"

type KubernetesEngineControlPlaneAssets struct {
	CACertPath                   string
	APIServerCertPath            string
	APIServerKeyPath             string
	SchedulerKubeconfigPath      string
	ControllerManagerConfigPath  string
	ServiceAccountPublicKeyPath  string
	ServiceAccountPrivateKeyPath string
}

type controlPlaneKubeconfig struct {
	APIVersion     string                     `yaml:"apiVersion"`
	Kind           string                     `yaml:"kind"`
	CurrentContext string                     `yaml:"current-context"`
	Clusters       []controlPlaneNamedCluster `yaml:"clusters"`
	Contexts       []controlPlaneNamedContext `yaml:"contexts"`
	Users          []controlPlaneNamedUser    `yaml:"users"`
}

type controlPlaneNamedCluster struct {
	Name    string                    `yaml:"name"`
	Cluster controlPlaneClusterConfig `yaml:"cluster"`
}

type controlPlaneClusterConfig struct {
	CertificateAuthority string `yaml:"certificate-authority"`
	Server               string `yaml:"server"`
}

type controlPlaneNamedContext struct {
	Name    string                    `yaml:"name"`
	Context controlPlaneContextConfig `yaml:"context"`
}

type controlPlaneContextConfig struct {
	Cluster string `yaml:"cluster"`
	User    string `yaml:"user"`
}

type controlPlaneNamedUser struct {
	Name string                 `yaml:"name"`
	User controlPlaneUserConfig `yaml:"user"`
}

type controlPlaneUserConfig struct {
	ClientCertificate string `yaml:"client-certificate"`
	ClientKey         string `yaml:"client-key"`
}

func EnsureKubernetesEngineControlPlaneAssets(pkiDir, configDir, clusterName, apiServerIP string, apiServerPort int) (KubernetesEngineControlPlaneAssets, error) {
	name, err := validateKubernetesEnginePkiClusterName(clusterName)
	if err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	clusterName = name
	if net.ParseIP(apiServerIP) == nil {
		return KubernetesEngineControlPlaneAssets{}, fmt.Errorf("invalid API server IP address %q", apiServerIP)
	}
	caCertPath, _, err := EnsureKubernetesEngineCA(pkiDir, clusterName)
	if err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	apiServerCertPath, apiServerKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "kube-apiserver",
		CommonName:    "kube-apiserver",
		Organizations: []string{"kubernetes"},
		Usage:         KubernetesEngineCertUsageServer,
		DNSNames:      []string{"localhost", "kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local"},
		IPAddresses:   []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP(apiServerIP)},
	})
	if err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	schedulerCertPath, schedulerKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "kube-scheduler",
		CommonName:    "system:kube-scheduler",
		Organizations: []string{"system:kube-scheduler"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	controllerCertPath, controllerKeyPath, err := IssueKubernetesEngineCertificate(pkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "kube-controller-manager",
		CommonName:    "system:kube-controller-manager",
		Organizations: []string{"system:kube-controller-manager"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	serviceAccountPublicKeyPath, serviceAccountPrivateKeyPath, err := EnsureKubernetesEngineServiceAccountKey(pkiDir, clusterName)
	if err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}

	clusterDir := filepath.Join(configDir, clusterName)
	if err := os.MkdirAll(clusterDir, 0o700); err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	serverURL := fmt.Sprintf("https://%s:%d", apiServerIP, apiServerPort)
	schedulerKubeconfigPath := filepath.Join(clusterDir, "kube-scheduler.kubeconfig")
	if err := writeControlPlaneKubeconfig(schedulerKubeconfigPath, serverURL, caCertPath, "system:kube-scheduler", schedulerCertPath, schedulerKeyPath); err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	controllerManagerConfigPath := filepath.Join(clusterDir, "kube-controller-manager.kubeconfig")
	if err := writeControlPlaneKubeconfig(controllerManagerConfigPath, serverURL, caCertPath, "system:kube-controller-manager", controllerCertPath, controllerKeyPath); err != nil {
		return KubernetesEngineControlPlaneAssets{}, err
	}
	return KubernetesEngineControlPlaneAssets{
		CACertPath:                   caCertPath,
		APIServerCertPath:            apiServerCertPath,
		APIServerKeyPath:             apiServerKeyPath,
		SchedulerKubeconfigPath:      schedulerKubeconfigPath,
		ControllerManagerConfigPath:  controllerManagerConfigPath,
		ServiceAccountPublicKeyPath:  serviceAccountPublicKeyPath,
		ServiceAccountPrivateKeyPath: serviceAccountPrivateKeyPath,
	}, nil
}

func writeControlPlaneKubeconfig(path, serverURL, caCertPath, userName, certPath, keyPath string) error {
	const clusterName = "mke"
	config := controlPlaneKubeconfig{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: userName + "@" + clusterName,
		Clusters: []controlPlaneNamedCluster{{Name: clusterName, Cluster: controlPlaneClusterConfig{
			CertificateAuthority: caCertPath,
			Server:               serverURL,
		}}},
		Contexts: []controlPlaneNamedContext{{Name: userName + "@" + clusterName, Context: controlPlaneContextConfig{
			Cluster: clusterName,
			User:    userName,
		}}},
		Users: []controlPlaneNamedUser{{Name: userName, User: controlPlaneUserConfig{
			ClientCertificate: certPath,
			ClientKey:         keyPath,
		}}},
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return writeControlPlaneFile(path, data, 0o600)
}

func writeControlPlaneFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTmp = false
	return nil
}
