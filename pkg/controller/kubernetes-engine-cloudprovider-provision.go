package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

// kubernetesEngineCloudControllerManagerUserName は cloud-controller-manager が
// kube-apiserverへ提示するクライアント証明書のCommonName/Organization。Kubernetesに組み込みの
// ClusterRoleBindingは存在しないため、EnsureKubernetesEngineCloudControllerManagerRBACが
// 同名のClusterRole/ClusterRoleBindingを明示的に適用してこの身元へ権限を付与する。
const kubernetesEngineCloudControllerManagerUserName = "system:cloud-controller-manager"

// kubernetesEngineCloudProviderApiKeyUserID は、CCM用APIKeyの発行対象ユーザー。mke-lb-controller
// 同様、専用システムユーザーは作成せず、bootstrap admin ユーザー配下でAPIKeyを発行する。
const kubernetesEngineCloudProviderApiKeyUserID = db.BootstrapAdminUserID

// issueKubernetesEngineCloudProviderApiKey は、CCMがmarmotd APIへアクセスするための永続APIKeyを
// bootstrap adminユーザー配下に発行し、キーIDと生トークンを返す。
func issueKubernetesEngineCloudProviderApiKey(database *db.Database, comment string) (string, string, error) {
	sessionType := "generated"
	apiKey, rawToken, err := database.CreateUserApiKey(kubernetesEngineCloudProviderApiKeyUserID, api.ApiKeyCreateRequest{
		Comment:     util.StringPtr(comment),
		SessionType: &sessionType,
	})
	if err != nil {
		return "", "", err
	}
	return apiKey.Metadata.Id, rawToken, nil
}

// revokeKubernetesEngineCloudProviderApiKey は、keyIDで発行済みのCCM用marmotd APIKeyを削除する。
// 既に失効済み(db.ErrNotFound)の場合は成功として扱う。
func revokeKubernetesEngineCloudProviderApiKey(database *db.Database, keyID string) error {
	if err := database.DeleteUserApiKey(kubernetesEngineCloudProviderApiKeyUserID, keyID); err != nil && err != db.ErrNotFound {
		return err
	}
	return nil
}

// EnsureKubernetesEngineCloudControllerManagerKubeconfig は、既存のクラスタ専用CAを使って
// system:cloud-controller-manager 証明書を発行し、kube-apiserverへの接続用kubeconfigを
// configDir/<clusterName>/cloud-controller-manager.kubeconfig に書き出す。
func EnsureKubernetesEngineCloudControllerManagerKubeconfig(pkiDir, configDir, clusterName, apiServerIP string, apiServerPort int) (string, error) {
	name, err := validateKubernetesEnginePkiClusterName(clusterName)
	if err != nil {
		return "", err
	}
	clusterName = name
	if apiServerPort <= 0 {
		return "", fmt.Errorf("invalid API server port %d", apiServerPort)
	}

	caCertPath, _, err := EnsureKubernetesEngineCA(pkiDir, clusterName)
	if err != nil {
		return "", err
	}
	certPath, keyPath, err := IssueKubernetesEngineCertificate(pkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "cloud-controller-manager",
		CommonName:    kubernetesEngineCloudControllerManagerUserName,
		Organizations: []string{kubernetesEngineCloudControllerManagerUserName},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return "", err
	}

	clusterDir := filepath.Join(configDir, clusterName)
	if err := os.MkdirAll(clusterDir, 0o700); err != nil {
		return "", err
	}
	kubeconfigPath := filepath.Join(clusterDir, "cloud-controller-manager.kubeconfig")
	serverURL := fmt.Sprintf("https://%s:%d", apiServerIP, apiServerPort)
	if err := writeControlPlaneKubeconfig(kubeconfigPath, serverURL, caCertPath, kubernetesEngineCloudControllerManagerUserName, certPath, keyPath); err != nil {
		return "", err
	}
	return kubeconfigPath, nil
}

const (
	// kubernetesEngineCloudProviderBinaryPath は、marmotdホスト上にインストール済みの
	// mke-node-controllerバイナリの配置場所(debパッケージにより/usr/local/marmot配下へ配置される)。
	// CCMはmke-lb-controllerと異なりmarmotdホスト自身のネットワーク名前空間内で動作するため、
	// 別サーバーへの配布(SCP等)は不要で、このパスをそのままsystemdユニットのExecStartに使う。
	kubernetesEngineCloudProviderBinaryPath = "/usr/local/marmot/mke-node-controller"

	kubernetesEngineCloudProviderApiKeyFileName = "cloud-controller-manager.apikey"
	kubernetesEngineCloudProviderCAFileName     = "cloud-controller-manager-marmotd-ca.pem"
)

// ProvisionKubernetesEngineCloudControllerManager は、marmotd APIKeyの発行、必要な設定ファイルの
// 書き出し、kubeconfig生成を行った上で、mke-node-controllerをCCM用systemdユニットとして起動する。
// mkeConf.CloudControllerManagerEnabled が true の場合のみ呼び出される任意導入機能である
// (フェーズ14項目4: kubeletの--cloud-provider=external切り替え、mke-lb-controllerの
// SetNodeAddresses呼び出し停止とあわせて有効化することを前提とする)。
func ProvisionKubernetesEngineCloudControllerManager(database *db.Database, configDir, pkiDir string, mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine, apiServerIP string, apiServerPort int) error {
	clusterName, err := validateKubernetesEngineEtcdClusterName(ke.Metadata.Name)
	if err != nil {
		return err
	}

	marmotdURL, err := buildKubernetesEngineLoadBalancerMarmotdURL()
	if err != nil {
		return fmt.Errorf("failed to build marmotd URL for cloud-controller-manager: %w", err)
	}

	keyID, rawToken, err := issueKubernetesEngineCloudProviderApiKey(database, fmt.Sprintf("mke-node-controller for %s", api.KubernetesEngineID(ke)))
	if err != nil {
		return fmt.Errorf("failed to issue cloud-controller-manager API key: %w", err)
	}
	if err := database.UpdateKubernetesEngineCloudProviderApiKeyID(api.KubernetesEngineID(ke), keyID); err != nil {
		return fmt.Errorf("failed to record cloud-controller-manager API key ID: %w", err)
	}

	clusterDir := filepath.Join(configDir, clusterName)
	if err := os.MkdirAll(clusterDir, 0o700); err != nil {
		return fmt.Errorf("failed to create cloud-controller-manager config dir: %w", err)
	}
	apiKeyPath := filepath.Join(clusterDir, kubernetesEngineCloudProviderApiKeyFileName)
	if err := os.WriteFile(apiKeyPath, []byte(rawToken), 0o600); err != nil {
		return fmt.Errorf("failed to write cloud-controller-manager API key file: %w", err)
	}

	extraArgs := []string{
		fmt.Sprintf("--marmotd-url=%s", marmotdURL),
		fmt.Sprintf("--marmotd-apikey-file=%s", apiKeyPath),
		fmt.Sprintf("--kubernetes-engine-id=%s", api.KubernetesEngineID(ke)),
		fmt.Sprintf("--internal-network=%s", kubernetesEngineNetworkName(ke)),
	}
	if kubernetesEngineLoadBalancerEnabled(ke) {
		extraArgs = append(extraArgs, fmt.Sprintf("--external-network=%s", kubernetesEngineLoadBalancerExternalNetwork))
	} else {
		// spec.nodeSpec.network.external=default のクラスタにはhost-bridge接続が無いため、
		// mke-node-controllerの既定値("host-bridge")を明示的に空文字列で上書きしてExternalIP設定を無効化する。
		extraArgs = append(extraArgs, "--external-network=")
	}
	if strings.TrimSpace(mkeConf.CloudProviderRegion) != "" {
		extraArgs = append(extraArgs, fmt.Sprintf("--region=%s", mkeConf.CloudProviderRegion))
	}
	if strings.TrimSpace(mkeConf.CloudProviderZone) != "" {
		extraArgs = append(extraArgs, fmt.Sprintf("--zone=%s", mkeConf.CloudProviderZone))
	}

	caPath, caData, err := buildKubernetesEngineLoadBalancerMarmotdCA()
	if err != nil {
		return fmt.Errorf("failed to load marmotd CA for cloud-controller-manager: %w", err)
	}
	if caPath != "" {
		destCAPath := filepath.Join(clusterDir, kubernetesEngineCloudProviderCAFileName)
		if err := os.WriteFile(destCAPath, caData, 0o600); err != nil {
			return fmt.Errorf("failed to write cloud-controller-manager CA file: %w", err)
		}
		extraArgs = append(extraArgs, fmt.Sprintf("--marmotd-ca-file=%s", destCAPath))
	}

	ccmKubeconfigPath, err := EnsureKubernetesEngineCloudControllerManagerKubeconfig(pkiDir, configDir, clusterName, apiServerIP, apiServerPort)
	if err != nil {
		return fmt.Errorf("failed to ensure cloud-controller-manager kubeconfig: %w", err)
	}

	return CreateKubernetesEngineCloudControllerManagerUnit(KubernetesEngineCloudControllerManagerUnitConfig{
		ClusterName:    clusterName,
		BinaryPath:     kubernetesEngineCloudProviderBinaryPath,
		KubeconfigPath: ccmKubeconfigPath,
		ExtraArgs:      extraArgs,
	})
}

// revokeKubernetesEngineCloudProviderApiKeyForEngine は、ke.Metadata.LabelsにCCM用APIKeyの
// IDが記録されていれば失効させる。CCM未導入クラスタ(記録無し)の場合は何もしない。
func revokeKubernetesEngineCloudProviderApiKeyForEngine(database *db.Database, ke api.KubernetesEngine) error {
	if ke.Metadata.Labels == nil {
		return nil
	}
	keyID, ok := (*ke.Metadata.Labels)[db.KubernetesEngineCloudProviderLabelApiKeyID].(string)
	if !ok || strings.TrimSpace(keyID) == "" {
		return nil
	}
	return revokeKubernetesEngineCloudProviderApiKey(database, keyID)
}
