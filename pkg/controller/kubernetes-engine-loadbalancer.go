package controller

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

const (
	// kubernetesEngineLoadBalancerRoleValue は kubernetesEngineNodeLabelRole ラベルに設定する
	// mke専用ロードバランサー用仮想サーバーのロール値。
	kubernetesEngineLoadBalancerRoleValue = "loadbalancer"
	// kubernetesEngineLoadBalancerLabelProvisioned は、kubeconfig配布まで完了したかどうかを示す
	// サーバーラベル(kubernetesEngineNodeLabelProvisionedと同種だが専用ロール分は独立管理する)。
	kubernetesEngineLoadBalancerLabelProvisioned = "kubernetesEngineLoadBalancerProvisioned"
	// kubernetesEngineLoadBalancerLabelApiKeyID は、mke-lb-controllerがmarmotd APIを呼び出す際に
	// 使用するAPIKeyのID(admin ユーザー配下)を保持するサーバーラベル。クラスタ削除時にこの
	// ラベルからAPIKeyを特定して失効させる。
	kubernetesEngineLoadBalancerLabelApiKeyID = "kubernetesEngineLoadBalancerApiKeyID"

	defaultKubernetesEngineLoadBalancerCPU    = 1
	defaultKubernetesEngineLoadBalancerMemory = 1024

	// kubernetesEngineLoadBalancerExternalNetwork は、フェーズ11の設計上の制約で、
	// LB連携がhost-bridge固定であることを表す(NATが必要なdefaultは使用しない)。
	kubernetesEngineLoadBalancerExternalNetwork = "host-bridge"

	// kubernetesEngineLoadBalancerKubeconfigPath は、LB仮想サーバー上でkube-apiserverへ
	// アクセスするためのkubeconfigの配置先。
	kubernetesEngineLoadBalancerKubeconfigPath = "/etc/marmot/mke-lb-kubeconfig"

	// kubernetesEngineLoadBalancerControllerBinaryDestPath は、LB仮想サーバー上に配置する
	// ロードバランサーコントローラー(mke-lb-controller)バイナリの配置先。
	kubernetesEngineLoadBalancerControllerBinaryDestPath = "/usr/local/bin/mke-lb-controller"
	// kubernetesEngineLoadBalancerHAProxyConfigPath は、ロードバランサーコントローラーが
	// 管理するHAProxy設定ファイルのパス。
	kubernetesEngineLoadBalancerHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	// kubernetesEngineLoadBalancerControllerSystemdUnitPath は、ロードバランサーコントローラーの
	// systemdユニットファイルの配置先。
	kubernetesEngineLoadBalancerControllerSystemdUnitPath = "/etc/systemd/system/mke-lb-controller.service"

	// kubernetesEngineLoadBalancerSysctlPath は、VIPがNICへ割り当てられる前でも
	// HAProxyがbindできるようにするnet.ipv4.ip_nonlocal_bind設定の配置先。
	kubernetesEngineLoadBalancerSysctlPath = "/etc/sysctl.d/99-marmot-mke-lb.conf"

	// kubernetesEngineLoadBalancerApiKeyPath は、mke-lb-controllerがmarmotd APIへアクセスする際に
	// 使用するAPIKeyトークン(生トークン文字列)の配置先。
	kubernetesEngineLoadBalancerApiKeyPath = "/etc/marmot/mke-lb-apikey"
	// kubernetesEngineLoadBalancerMarmotdCAPath は、mke-lb-controllerがmarmotd APIへHTTPSアクセス
	// する際に信頼する証明書群の配置先。
	kubernetesEngineLoadBalancerMarmotdCAPath = "/etc/marmot/mke-lb-marmotd-ca.pem"

	// kubernetesEngineLoadBalancerApiKeyUserID は、mke-lb-controller用APIKeyの発行対象ユーザー。
	// 専用システムユーザーは作成せず、bootstrap admin ユーザー配下でAPIKeyを発行する。
	kubernetesEngineLoadBalancerApiKeyUserID = db.BootstrapAdminUserID
)

// kubernetesEngineLoadBalancerControllerBinarySourcePath は、marmotdホスト上にインストール済みの
// mke-lb-controllerバイナリの読み込み元。marmot-lb-agent等と同様、debパッケージにより
// /usr/local/marmot 配下へ配置される前提。
var kubernetesEngineLoadBalancerControllerBinarySourcePath = "/usr/local/marmot/mke-lb-controller"

var runKubernetesEngineLoadBalancerProvision = provisionKubernetesEngineLoadBalancerSSH

// kubernetesEngineLoadBalancerEnabled は、spec.nodeSpec.network.external が host-bridge の
// 場合のみ mke専用ロードバランサーを有効とする(defaultではNATが必要になるため使用しない)。
func kubernetesEngineLoadBalancerEnabled(ke api.KubernetesEngine) bool {
	external := "default"
	if ke.Spec.NodeSpec != nil && ke.Spec.NodeSpec.Network != nil && ke.Spec.NodeSpec.Network.External != nil {
		external = strings.TrimSpace(*ke.Spec.NodeSpec.Network.External)
	}
	return external == kubernetesEngineLoadBalancerExternalNetwork
}

// kubernetesEngineLoadBalancerServerName はクラスタ専用のmkeロードバランサー用仮想サーバー名。
func kubernetesEngineLoadBalancerServerName(ke api.KubernetesEngine) string {
	return fmt.Sprintf("mke-%s-lb", strings.TrimSpace(ke.Metadata.Name))
}

// findKubernetesEngineLoadBalancerServer は、このクラスタが所有するmkeロードバランサー用
// 仮想サーバーを返す(存在しない場合はnil,nil)。
func findKubernetesEngineLoadBalancerServer(database *db.Database, ke api.KubernetesEngine) (*api.Server, error) {
	servers, err := database.GetServers()
	if err != nil {
		return nil, err
	}
	owner := api.KubernetesEngineID(ke)
	var found []api.Server
	for _, server := range servers {
		if server.Metadata.Labels == nil {
			continue
		}
		labels := *server.Metadata.Labels
		if labels[kubernetesEngineNodeLabelOwner] == owner && labels[kubernetesEngineNodeLabelRole] == kubernetesEngineLoadBalancerRoleValue {
			found = append(found, server)
		}
	}
	if len(found) == 0 {
		return nil, nil
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Metadata.Name < found[j].Metadata.Name })
	return &found[0], nil
}

// buildKubernetesEngineLoadBalancerServerSpec は、mkeロードバランサー用仮想サーバーのspecを
// 組み立てる。フェーズ11の設計により、外部アクセス用ネットワークはhost-bridge固定とし、
// ノード間通信用ネットワークにも接続してkube-apiserverへ到達できるようにする。
func buildKubernetesEngineLoadBalancerServerSpec(ke api.KubernetesEngine, publicKey string) (api.Server, error) {
	if strings.TrimSpace(publicKey) == "" {
		return api.Server{}, fmt.Errorf("KubernetesEngine load balancer public key is empty")
	}
	name := kubernetesEngineLoadBalancerServerName(ke)
	labels := map[string]interface{}{
		kubernetesEngineNodeLabelOwner: api.KubernetesEngineID(ke),
		kubernetesEngineNodeLabelRole:  kubernetesEngineLoadBalancerRoleValue,
	}
	metadata := api.Metadata{Name: name, Labels: &labels}
	if ke.Metadata.NodeName != nil && strings.TrimSpace(*ke.Metadata.NodeName) != "" {
		metadata.NodeName = util.StringPtr(strings.TrimSpace(*ke.Metadata.NodeName))
	}
	nics := []api.NetworkInterface{
		{Networkname: kubernetesEngineLoadBalancerExternalNetwork},
		{Networkname: kubernetesEngineNetworkName(ke)},
	}
	auth, err := mergeKubernetesEngineNodeAuth(nil, publicKey)
	if err != nil {
		return api.Server{}, err
	}
	return api.Server{
		ApiVersion: "v1",
		Kind:       "Server",
		Metadata:   metadata,
		Spec: api.ServerSpec{
			Cpu:              util.IntPtrInt(defaultKubernetesEngineLoadBalancerCPU),
			Memory:           util.IntPtrInt(defaultKubernetesEngineLoadBalancerMemory),
			MmImage:          util.StringPtr("ubuntu24.04"),
			NetworkInterface: &nics,
			Auth:             auth,
		},
	}, nil
}

// ProvisionKubernetesEngineLoadBalancer は mke専用ロードバランサー仮想サーバーを冪等に用意し、
// HA-Proxyとロードバランサーコントローラー(mke-lb-controller)をインストールした上で、
// kube-apiserverへアクセスするためのkubeconfigを配布する。external=defaultの場合は
// フェーズ11の設計制約によりロードバランサーを使用しないため、常にreadyとして扱う。
// 戻り値はプロビジョニングが完了した(true)かどうかを表す。VIP払い出し・内部DNS登録用の
// marmotd API連携は別変更セットで実装する。
func ProvisionKubernetesEngineLoadBalancer(database *db.Database, mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) (bool, error) {
	if !kubernetesEngineLoadBalancerEnabled(ke) {
		return true, nil
	}
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return false, fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}

	server, err := findKubernetesEngineLoadBalancerServer(database, ke)
	if err != nil {
		return false, err
	}
	if server == nil {
		publicKey, readErr := os.ReadFile(kubernetesEngineNodePublicKeyPath)
		if readErr != nil {
			return false, fmt.Errorf("failed to read KubernetesEngine load balancer public key: %w", readErr)
		}
		spec, buildErr := buildKubernetesEngineLoadBalancerServerSpec(ke, strings.TrimSpace(string(publicKey)))
		if buildErr != nil {
			return false, buildErr
		}
		if _, createErr := database.MakeServerEntry(spec); createErr != nil {
			return false, fmt.Errorf("failed to create KubernetesEngine load balancer server: %w", createErr)
		}
		return false, nil
	}

	if server.Status == nil {
		return false, nil
	}
	switch server.Status.StatusCode {
	case db.SERVER_RUNNING:
	case db.SERVER_ERROR:
		message := "load balancer server entered error state"
		if server.Status.Message != nil && strings.TrimSpace(*server.Status.Message) != "" {
			message = strings.TrimSpace(*server.Status.Message)
		}
		return false, fmt.Errorf("load balancer server %s: %s", server.Metadata.Name, message)
	default:
		return false, nil
	}

	labels := make(map[string]interface{})
	if server.Metadata.Labels != nil {
		labels = *server.Metadata.Labels
	}
	if labels[kubernetesEngineLoadBalancerLabelProvisioned] == "true" {
		return true, nil
	}

	nodeIP, err := kubernetesEngineNodeInternalIP(*server, kubernetesEngineNetworkName(ke))
	if err != nil {
		return false, err
	}
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	kubeconfig, err := EnsureKubernetesEngineAdminKubeconfig(DefaultKubernetesEnginePkiDir, clusterName, *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)
	if err != nil {
		return false, fmt.Errorf("failed to issue load balancer kubeconfig: %w", err)
	}
	controllerBinary, err := os.ReadFile(kubernetesEngineLoadBalancerControllerBinarySourcePath)
	if err != nil {
		return false, fmt.Errorf("failed to read mke-lb-controller binary: %w", err)
	}
	marmotdURL, err := buildKubernetesEngineLoadBalancerMarmotdURL()
	if err != nil {
		return false, fmt.Errorf("failed to resolve marmotd API URL for load balancer: %w", err)
	}
	keyID, apiKeyToken, err := issueKubernetesEngineLoadBalancerApiKey(database, resourceIDForApiKeyComment(ke))
	if err != nil {
		return false, fmt.Errorf("failed to issue load balancer marmotd API key: %w", err)
	}
	apiKeyCommitted := false
	defer func() {
		if apiKeyCommitted {
			return
		}
		if delErr := database.DeleteUserApiKey(kubernetesEngineLoadBalancerApiKeyUserID, keyID); delErr != nil && delErr != db.ErrNotFound {
			slog.Warn("failed to revoke load balancer API key after provisioning error", "key_id", keyID, "err", delErr)
		}
	}()
	marmotdCAFile, marmotdCAData, err := buildKubernetesEngineLoadBalancerMarmotdCA()
	if err != nil {
		return false, fmt.Errorf("failed to resolve marmotd CA bundle for load balancer: %w", err)
	}
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return false, err
	}
	resourceID := fmt.Sprintf("%s-lb", api.KubernetesEngineID(ke))
	if err := runKubernetesEngineLoadBalancerProvision(nodeIP, kubernetesEngineNodePrivateKeyPath, namespace, resourceID, kubeconfig, controllerBinary, apiKeyToken, marmotdURL, marmotdCAFile, marmotdCAData, api.KubernetesEngineID(ke), mkeConf.CloudControllerManagerEnabled); err != nil {
		return false, fmt.Errorf("failed to provision load balancer: %w", err)
	}

	labels[kubernetesEngineLoadBalancerLabelProvisioned] = "true"
	labels[kubernetesEngineLoadBalancerLabelApiKeyID] = keyID
	if err := database.UpdateServer(server.Metadata.Id, api.Server{Metadata: api.Metadata{Labels: &labels}}); err != nil {
		return false, fmt.Errorf("failed to mark load balancer server as provisioned: %w", err)
	}
	apiKeyCommitted = true
	return true, nil
}

// buildKubernetesEngineLoadBalancerMarmotdURL は、mke-lb-controllerがmarmotd APIへアクセスする
// ためのベースURLを組み立てる。marmotdホスト自身のhost-bridge接続アドレスは自動検出せず、
// marmotd.json の api-advertise-host-bridge-address で明示指定する運用とする。
func buildKubernetesEngineLoadBalancerMarmotdURL() (string, error) {
	cfg := marmotd.CurrentConfig()
	host := strings.TrimSpace(cfg.APIAdvertiseHostBridgeAddress)
	if host == "" {
		return "", fmt.Errorf("marmotd.json api-advertise-host-bridge-address is not configured")
	}
	port := "8750"
	if _, p, err := net.SplitHostPort(strings.TrimSpace(cfg.APIListenAddr)); err == nil && strings.TrimSpace(p) != "" {
		port = p
	}
	scheme := "http"
	if strings.TrimSpace(cfg.TLSCertFile) != "" && strings.TrimSpace(cfg.TLSKeyFile) != "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port), nil
}

func buildKubernetesEngineLoadBalancerMarmotdCA() (string, []byte, error) {
	cfg := marmotd.CurrentConfig()
	if strings.TrimSpace(cfg.TLSCertFile) == "" || strings.TrimSpace(cfg.TLSKeyFile) == "" {
		return "", nil, nil
	}
	certData, err := os.ReadFile(cfg.TLSCertFile)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read marmotd TLS certificate %q: %w", cfg.TLSCertFile, err)
	}
	return kubernetesEngineLoadBalancerMarmotdCAPath, certData, nil
}

// resourceIDForApiKeyComment は発行するAPIKeyのコメントに残すクラスタ識別子。
func resourceIDForApiKeyComment(ke api.KubernetesEngine) string {
	return fmt.Sprintf("mke-lb-controller for %s", api.KubernetesEngineID(ke))
}

// issueKubernetesEngineLoadBalancerApiKey は、mke-lb-controllerがmarmotd APIへアクセスするための
// 永続APIKeyをbootstrap adminユーザー配下に発行し、キーIDと生トークンを返す。
func issueKubernetesEngineLoadBalancerApiKey(database *db.Database, comment string) (string, string, error) {
	sessionType := "generated"
	apiKey, rawToken, err := database.CreateUserApiKey(kubernetesEngineLoadBalancerApiKeyUserID, api.ApiKeyCreateRequest{
		Comment:     util.StringPtr(comment),
		SessionType: &sessionType,
	})
	if err != nil {
		return "", "", err
	}
	return apiKey.Metadata.Id, rawToken, nil
}

// revokeKubernetesEngineLoadBalancerApiKey は、LBサーバーに発行済みのmarmotd APIKeyがあれば
// 削除する。サーバーラベルにAPIKey IDが記録されていない場合は何もしない。
func revokeKubernetesEngineLoadBalancerApiKey(database *db.Database, server api.Server) {
	if server.Metadata.Labels == nil {
		return
	}
	labels := *server.Metadata.Labels
	keyID, ok := labels[kubernetesEngineLoadBalancerLabelApiKeyID].(string)
	if !ok || strings.TrimSpace(keyID) == "" {
		return
	}
	if err := database.DeleteUserApiKey(kubernetesEngineLoadBalancerApiKeyUserID, keyID); err != nil && err != db.ErrNotFound {
		slog.Warn("revokeKubernetesEngineLoadBalancerApiKey: DeleteUserApiKey() failed",
			"serverId", api.ServerID(server), "key_id", keyID, "err", err)
	}
}
