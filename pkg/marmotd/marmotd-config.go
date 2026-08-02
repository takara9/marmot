package marmotd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/takara9/marmot/pkg/db"
)

const DefaultConfigPath = "/etc/marmot/marmotd.json"

const (
	DefaultCephConfPath    = "/etc/ceph/ceph.conf"
	DefaultCephKeyringPath = "/etc/ceph/ceph.client.admin.keyring"
)

var cephConfPathForRuntime = DefaultCephConfPath
var cephKeyringPathForRuntime = DefaultCephKeyringPath

func effectiveCephConfPath() string {
	if path := strings.TrimSpace(cephConfPathForRuntime); path != "" {
		return path
	}
	return DefaultCephConfPath
}

func effectiveCephKeyringPath() string {
	if path := strings.TrimSpace(cephKeyringPathForRuntime); path != "" {
		return path
	}
	return DefaultCephKeyringPath
}

// OSImage represents an OS image configuration to be automatically provisioned
type OSImage struct {
	// イメージの名前
	// 例: "ubuntu22.04"
	Name string `json:"name"`

	// イメージの URL
	// 例: "https://cloud-images.ubuntu.com/releases/jammy/release/ubuntu-22.04-server-cloudimg-amd64.img"
	URL string `json:"url"`

	// OS の名前
	// 例: "ubuntu"
	OSName string `json:"osName"`

	// OS のバージョン
	// 例: "22.04"
	OSVersion string `json:"osVersion"`
}

type HostBridgeRouteConfig struct {
	To  string `json:"to"`
	Via string `json:"via"`
}

type HostBridgeNameserversConfig struct {
	Addresses []string `json:"addresses"`
	Search    []string `json:"search"`
}

type HostBridgeDefaultConfig struct {
	Netmasklen  int                         `json:"netmasklen"`
	Nameservers HostBridgeNameserversConfig `json:"nameservers"`
	Routes      []HostBridgeRouteConfig     `json:"routes"`
}

type cephPoolByClassEntry struct {
	StorageClass string `json:"storageClass"`
	Pool         string `json:"pool"`
}

// MarmotdConfig は /etc/marmot/marmotd.json で設定可能なパラメータを保持します。
type MarmotdConfig struct {
	// ハイパーバイザーのノード名
	// 例: "hv1"
	NodeName string `json:"node_name"`

	// etcd のエンドポイント URL
	// 例: "http://127.0.0.1:2379"
	EtcdURL string `json:"etcd_url"`

	// marmot-API サーバーのバインドアドレスとポート番号
	// 例: "0.0.0.0:8750"
	APIListenAddr string `json:"api_listen_addr"`

	// internal-DNS サーバーのバインドアドレスとポート番号
	// 例: "127.0.0.1:53"
	DNSListenAddr string `json:"dns_listen_addr"`

	// DNS 外部参照先アドレス (フォワーダー)
	// 例: "8.8.8.8:53"
	DNSUpstream string `json:"dns_upstream"`

	// upstream DNS 転送を許可する送信元 CIDR の一覧
	// 例: ["192.168.1.0/24", "fd00::/64"]
	DNSUpstreamAllowCIDRs []string `json:"dns_client_allow_cidrs"`

	// VXLAN 利用時に underlayInterface が省略された場合の既定インターフェース名
	DefaultUnderlayInterface string `json:"default_underlay_interface"`

	// OS ボリューム用の LVM Volume Group 名
	// 例: "vg1"
	OSVolumeGroup string `json:"os_volume_group"`

	// DATA ボリューム用の LVM Volume Group 名
	// 例: "vg2"
	DataVolumeGroup string `json:"data_volume_group"`

	// host-bridge の IP アドレス範囲を表す CIDR
	// 例: "192.168.1.0/24"
	HostBridgeIPNetAddr string `json:"host-bridge-ip-net-addr"`

	// host-bridge の IP アドレス払い出し開始アドレス
	// 例: "192.168.1.190"
	HostBridgeIPAddrStart string `json:"host-bridge-ip-addr-start"`

	// host-bridge の IP アドレス払い出し終了アドレス
	// 例: "192.168.1.194"
	HostBridgeIPAddrEnd string `json:"host-bridge-ip-addr-end"`

	// host-bridge 利用時のサーバー既定ネットワーク設定
	HostBridgeDefault *HostBridgeDefaultConfig `json:"host-bridge-default"`

	// コントローラーが DeletionTimestamp を検知してから実際に削除処理を
	// 開始するまでの待機秒数
	DeletionDelaySeconds int `json:"deletion_delay_seconds"`

	// ログインセッションをアイドル判定で失効させるまでの時間。
	// "<数値><単位>" 形式で指定し、単位は m / h / d を利用可能。
	// 例: "30m", "24h", "3d"
	SessionIdleTimeout string `json:"session_idle_timeout"`

	// 実行中 VM からイメージを作成する際の既定タイムアウト秒数
	ImageCreateFromVMTimeoutSeconds int `json:"image_create_from_vm_timeout_seconds"`

	// URL からイメージを作成する際の既定タイムアウト秒数
	ImageCreateFromURLTimeoutSeconds int `json:"image_create_from_url_timeout_seconds"`

	// URL からイメージをダウンロードする際のタイムアウト秒数
	ImageDownloadTimeoutSeconds int `json:"image_download_timeout_seconds"`

	// QCOW2 イメージ拡張処理のタイムアウト秒数
	ImageResizeTimeoutSeconds int `json:"image_resize_timeout_seconds"`

	// イメージ削除処理のタイムアウト秒数
	ImageDeleteTimeoutSeconds int `json:"image_delete_timeout_seconds"`

	// Ceph ボリューム作成・削除処理のタイムアウト秒数
	CephVolumeOperationTimeoutSeconds int `json:"ceph_volume_operation_timeout_seconds"`

	// このホストが iSCSI ターゲットサーバーを担当するかどうか
	// true の場合、このホストの volumeコントローラーが iSCSI ターゲットを管理する。
	// false（省略時）の場合、クラスタ内で HostId が最小のホストが自動的に担当する。
	IscsiServer bool `json:"iscsi_server"`

	// 起動時に自動ダウンロード・登録する OS イメージの一覧
	// marmotd 起動時に各イメージをチェックし、存在しなければ登録する
	OSImages []OSImage `json:"os_images"`

	// Loki ログ受信用の Push API エンドポイント URL。
	// 例: "http://127.0.0.1:3100/loki/api/v1/push"
	LokiPushURL string `json:"loki_push_url"`

	// API サーバーが HTTPS を使用する場合の TLS 証明書ファイルパス。
	// 例: "/etc/marmot/certs/server.crt"
	// 空の場合は HTTP を使用する。
	TLSCertFile string `json:"tls_cert_file"`

	// API サーバーが HTTPS を使用する場合の TLS 秘密鍵ファイルパス。
	// 例: "/etc/marmot/certs/server.key"
	// 空の場合は HTTP を使用する。
	TLSKeyFile string `json:"tls_key_file"`

	// Ceph 連携の有効/無効フラグ。
	// true の場合、Ceph をバックエンドストレージとして利用可能にする。
	// false（省略時）の場合、Ceph 機能は無効化される。
	CephEnabled bool `json:"ceph_enabled"`

	// storageClass から Ceph CRUSH rule への対応マップ。
	// キーは storageClass (hdd, ssd, nvme など)、値は CRUSH rule 名。
	// 例: {"hdd": "rule-hdd", "ssd": "rule-ssd", "nvme": "rule-nvme"}
	CephCrushRuleByClass map[string]string `json:"ceph_crush_rule_by_class"`

	// storageClass から Ceph pool への対応マップ。
	// marmotd.json では配列形式で指定する:
	// 例: [{"storageClass":"hdd","pool":"marmot-hdd"},{"storageClass":"ssd","pool":"marmot-ssd"}]
	// 旧 map 形式はサポートしない。
	CephPoolByClass map[string]string `json:"ceph_pool_by_class"`
}

func (c *MarmotdConfig) UnmarshalJSON(data []byte) error {
	type alias MarmotdConfig
	aux := struct {
		*alias
		CephPoolByClass json.RawMessage `json:"ceph_pool_by_class"`
		// Backward compatibility for renamed DNS allow-list key.
		DNSUpstreamAllowCIDRsLegacy *[]string `json:"dns_upstream_allow_cidrs"`
		// Keep accepting issue-title typo for compatibility.
		DNSClientAllowCIDERSTypo *[]string `json:"dns_client_allow_ciders"`
		DNSClientAllowCIDRs     *[]string `json:"dns_client_allow_cidrs"`
	}{
		alias: (*alias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Prefer the canonical key, then typo compatibility key, then legacy key.
	if aux.DNSClientAllowCIDRs != nil {
		c.DNSUpstreamAllowCIDRs = *aux.DNSClientAllowCIDRs
	} else if aux.DNSClientAllowCIDERSTypo != nil {
		c.DNSUpstreamAllowCIDRs = *aux.DNSClientAllowCIDERSTypo
	} else if aux.DNSUpstreamAllowCIDRsLegacy != nil {
		c.DNSUpstreamAllowCIDRs = *aux.DNSUpstreamAllowCIDRsLegacy
	}

	raw := strings.TrimSpace(string(aux.CephPoolByClass))
	if raw == "" || raw == "null" {
		return nil
	}

	var entries []cephPoolByClassEntry
	if err := json.Unmarshal(aux.CephPoolByClass, &entries); err != nil {
		return fmt.Errorf("ceph_pool_by_class: must be an array of objects with storageClass and pool")
	}

	parsed := make(map[string]string, len(entries))
	for _, entry := range entries {
		storageClass := strings.TrimSpace(entry.StorageClass)
		pool := strings.TrimSpace(entry.Pool)
		if storageClass == "" || pool == "" {
			continue
		}
		parsed[storageClass] = pool
	}
	c.CephPoolByClass = parsed

	return nil
}

var runtimeConfigState = struct {
	mu  sync.RWMutex
	cfg *MarmotdConfig
}{
	cfg: defaultConfig(),
}

var listInterfaces = net.Interfaces
var listInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// defaultConfig はコンフィグファイルが存在しない場合や一部フィールドが
// 指定されていない場合に使用されるデフォルト値を返します。
func defaultConfig() *MarmotdConfig {
	return &MarmotdConfig{
		NodeName:                          "hv1",
		EtcdURL:                           "http://127.0.0.1:2379",
		APIListenAddr:                     "0.0.0.0:8750",
		DNSListenAddr:                     "0.0.0.0:53",
		DNSUpstream:                       "8.8.8.8:53",
		DNSUpstreamAllowCIDRs:             nil,
		DefaultUnderlayInterface:          "",
		OSVolumeGroup:                     db.DefaultOSVolumeGroup,
		DataVolumeGroup:                   db.DefaultDataVolumeGroup,
		DeletionDelaySeconds:              10,
		SessionIdleTimeout:                "1h",
		ImageCreateFromVMTimeoutSeconds:   600,
		ImageCreateFromURLTimeoutSeconds:  1800,
		ImageDownloadTimeoutSeconds:       1800,
		ImageResizeTimeoutSeconds:         600,
		ImageDeleteTimeoutSeconds:         120,
		CephVolumeOperationTimeoutSeconds: 120,
		LokiPushURL:                       "",
		TLSCertFile:                       "",
		TLSKeyFile:                        "",
		CephEnabled:                       false,
		CephCrushRuleByClass:              make(map[string]string),
		CephPoolByClass:                   make(map[string]string),
	}
}

func init() {
	SetRuntimeConfig(defaultConfig())
}

func normalizeConfig(cfg *MarmotdConfig) *MarmotdConfig {
	normalized := defaultConfig()
	if cfg == nil {
		return normalized
	}

	*normalized = *cfg
	defaults := defaultConfig()
	if strings.TrimSpace(normalized.NodeName) == "" {
		normalized.NodeName = defaults.NodeName
	}
	if strings.TrimSpace(normalized.EtcdURL) == "" {
		normalized.EtcdURL = defaults.EtcdURL
	}
	if strings.TrimSpace(normalized.APIListenAddr) == "" {
		normalized.APIListenAddr = defaults.APIListenAddr
	}
	if strings.TrimSpace(normalized.DNSListenAddr) == "" {
		if resolved, ok := resolveDNSListenAddrFromInterfaces(); ok {
			normalized.DNSListenAddr = resolved
		} else {
			normalized.DNSListenAddr = defaults.DNSListenAddr
		}
	}
	if strings.TrimSpace(normalized.DNSUpstream) == "" {
		normalized.DNSUpstream = defaults.DNSUpstream
	}
	if len(normalized.DNSUpstreamAllowCIDRs) > 0 {
		allowed := normalized.DNSUpstreamAllowCIDRs[:0]
		for _, cidr := range normalized.DNSUpstreamAllowCIDRs {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			allowed = append(allowed, cidr)
		}
		normalized.DNSUpstreamAllowCIDRs = allowed
	}
	normalized.DefaultUnderlayInterface = strings.TrimSpace(normalized.DefaultUnderlayInterface)
	if strings.TrimSpace(normalized.OSVolumeGroup) == "" {
		normalized.OSVolumeGroup = db.DefaultOSVolumeGroup
	}
	if strings.TrimSpace(normalized.DataVolumeGroup) == "" {
		normalized.DataVolumeGroup = db.DefaultDataVolumeGroup
	}
	normalized.HostBridgeIPNetAddr = strings.TrimSpace(normalized.HostBridgeIPNetAddr)
	normalized.HostBridgeIPAddrStart = strings.TrimSpace(normalized.HostBridgeIPAddrStart)
	normalized.HostBridgeIPAddrEnd = strings.TrimSpace(normalized.HostBridgeIPAddrEnd)
	if normalized.HostBridgeDefault != nil {
		normalized.HostBridgeDefault.Nameservers.Addresses = trimNonEmptyStrings(normalized.HostBridgeDefault.Nameservers.Addresses)
		normalized.HostBridgeDefault.Nameservers.Search = trimNonEmptyStrings(normalized.HostBridgeDefault.Nameservers.Search)
		for i := range normalized.HostBridgeDefault.Routes {
			normalized.HostBridgeDefault.Routes[i].To = strings.TrimSpace(normalized.HostBridgeDefault.Routes[i].To)
			normalized.HostBridgeDefault.Routes[i].Via = strings.TrimSpace(normalized.HostBridgeDefault.Routes[i].Via)
		}
	}
	if normalized.DeletionDelaySeconds <= 0 {
		normalized.DeletionDelaySeconds = defaults.DeletionDelaySeconds
	}
	normalized.SessionIdleTimeout = strings.TrimSpace(normalized.SessionIdleTimeout)
	if normalized.SessionIdleTimeout == "" {
		normalized.SessionIdleTimeout = defaults.SessionIdleTimeout
	}
	if normalized.ImageCreateFromVMTimeoutSeconds <= 0 {
		normalized.ImageCreateFromVMTimeoutSeconds = defaults.ImageCreateFromVMTimeoutSeconds
	}
	if normalized.ImageCreateFromURLTimeoutSeconds <= 0 {
		normalized.ImageCreateFromURLTimeoutSeconds = defaults.ImageCreateFromURLTimeoutSeconds
	}
	if normalized.ImageDownloadTimeoutSeconds <= 0 {
		normalized.ImageDownloadTimeoutSeconds = defaults.ImageDownloadTimeoutSeconds
	}
	if normalized.ImageResizeTimeoutSeconds <= 0 {
		normalized.ImageResizeTimeoutSeconds = defaults.ImageResizeTimeoutSeconds
	}
	if normalized.ImageDeleteTimeoutSeconds <= 0 {
		normalized.ImageDeleteTimeoutSeconds = defaults.ImageDeleteTimeoutSeconds
	}
	if normalized.CephVolumeOperationTimeoutSeconds <= 0 {
		normalized.CephVolumeOperationTimeoutSeconds = defaults.CephVolumeOperationTimeoutSeconds
	}
	normalized.LokiPushURL = strings.TrimSpace(normalized.LokiPushURL)
	normalized.TLSCertFile = strings.TrimSpace(normalized.TLSCertFile)
	normalized.TLSKeyFile = strings.TrimSpace(normalized.TLSKeyFile)

	// Ceph マップのキーと値をトリミング
	if normalized.CephCrushRuleByClass == nil {
		normalized.CephCrushRuleByClass = make(map[string]string)
	} else {
		normalized.CephCrushRuleByClass = trimMapStringString(normalized.CephCrushRuleByClass)
	}
	if normalized.CephPoolByClass == nil {
		normalized.CephPoolByClass = make(map[string]string)
	} else {
		normalized.CephPoolByClass = trimMapStringString(normalized.CephPoolByClass)
	}

	return normalized
}

func resolveDNSListenAddrFromInterfaces() (string, bool) {
	ifaces, err := listInterfaces()
	if err != nil {
		return "", false
	}

	sort.Slice(ifaces, func(i, j int) bool {
		return ifaces[i].Index < ifaces[j].Index
	})

	startIdx := 0
	for i, iface := range ifaces {
		if iface.Name == "lo" {
			startIdx = i + 1
			break
		}
	}

	for i := startIdx; i < len(ifaces); i++ {
		iface := ifaces[i]
		addrs, err := listInterfaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ip := extractIPv4(addr); ip != nil {
				return net.JoinHostPort(ip.String(), "53"), true
			}
		}
	}

	return "", false
}

func extractIPv4(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() {
			return nil
		}
		return ip
	case *net.IPAddr:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() {
			return nil
		}
		return ip
	default:
		return nil
	}
}

func trimNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	trimmed := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	return trimmed
}

func trimMapStringString(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	trimmed := make(map[string]string)
	for k, v := range m {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			trimmed[k] = v
		}
	}
	return trimmed
}

func (c *MarmotdConfig) ImageCreateFromVMTimeout() time.Duration {
	return time.Duration(c.ImageCreateFromVMTimeoutSeconds) * time.Second
}

func (c *MarmotdConfig) ImageCreateFromURLTimeout() time.Duration {
	return time.Duration(c.ImageCreateFromURLTimeoutSeconds) * time.Second
}

func (c *MarmotdConfig) ImageDownloadTimeout() time.Duration {
	return time.Duration(c.ImageDownloadTimeoutSeconds) * time.Second
}

func (c *MarmotdConfig) ImageResizeTimeout() time.Duration {
	return time.Duration(c.ImageResizeTimeoutSeconds) * time.Second
}

func (c *MarmotdConfig) ImageDeleteTimeout() time.Duration {
	return time.Duration(c.ImageDeleteTimeoutSeconds) * time.Second
}

func (c *MarmotdConfig) CephVolumeOperationTimeout() time.Duration {
	return time.Duration(c.CephVolumeOperationTimeoutSeconds) * time.Second
}

func SetRuntimeConfig(cfg *MarmotdConfig) {
	normalized := normalizeConfig(cfg)
	sessionIdleTimeout, err := parseSessionIdleTimeout(normalized.SessionIdleTimeout)
	if err != nil {
		panic(fmt.Sprintf("invalid runtime session_idle_timeout: %v", err))
	}
	if err := db.SetAuthSessionIdleTimeout(sessionIdleTimeout); err != nil {
		panic(fmt.Sprintf("failed to set auth session idle timeout: %v", err))
	}

	runtimeConfigState.mu.Lock()
	runtimeConfigState.cfg = normalized
	runtimeConfigState.mu.Unlock()

	db.SetDefaultVolumeGroups(normalized.OSVolumeGroup, normalized.DataVolumeGroup)
}

func CurrentConfig() *MarmotdConfig {
	runtimeConfigState.mu.RLock()
	defer runtimeConfigState.mu.RUnlock()
	copy := *runtimeConfigState.cfg
	return &copy
}

// LoadConfig は path で指定された JSON ファイルを読み込み MarmotdConfig を返します。
// ファイルが存在しない場合はデフォルト値を持つ設定を返します。
// ファイルが存在するが一部フィールドが省略されている場合は、
// デフォルト値で補完されます。
func LoadConfig(path string) (*MarmotdConfig, error) {
	cfg := defaultConfig()

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		// ファイルが存在しない場合はデフォルト設定を返す
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, err
	}

	normalized := normalizeConfig(cfg)
	if _, err := parseSessionIdleTimeout(normalized.SessionIdleTimeout); err != nil {
		return nil, err
	}
	if err := validateCephConfig(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

const (
	// SessionIdleTimeoutMaxMinutes は session_idle_timeout に指定できる最大値（分）です。
	// 7日間 = 10080分 を上限とします。
	SessionIdleTimeoutMaxMinutes = 10080
)

func parseSessionIdleTimeout(v string) (time.Duration, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0, nil
	}
	if len(s) < 2 {
		return 0, fmt.Errorf("session_idle_timeout format is invalid: %q", v)
	}

	// 末尾単位を大小文字非依存で扱う（" 2H " などを許容）
	unit := strings.ToLower(s[len(s)-1:])[0]
	num := s[:len(s)-1]

	count, err := strconv.ParseInt(num, 10, 64)
	if err != nil || count <= 0 {
		return 0, fmt.Errorf("session_idle_timeout format is invalid: %q", v)
	}

	const maxMinutes int64 = 7 * 24 * 60

	var minutes int64
	switch unit {
	case 'm':
		if count > maxMinutes {
			return 0, fmt.Errorf("session_idle_timeout must be <= 7d: %q", v)
		}
		minutes = count
	case 'h':
		if count > 7*24 {
			return 0, fmt.Errorf("session_idle_timeout must be <= 7d: %q", v)
		}
		minutes = count * 60
	case 'd':
		if count > 7 {
			return 0, fmt.Errorf("session_idle_timeout must be <= 7d: %q", v)
		}
		minutes = count * 24 * 60
	default:
		return 0, fmt.Errorf("session_idle_timeout unit is invalid: %q", v)
	}

	return time.Duration(minutes) * time.Minute, nil
}

// validateCephConfig は ceph_enabled=true の場合に必須項目を検証します。
func validateCephConfig(cfg *MarmotdConfig) error {
	if !cfg.CephEnabled {
		return nil
	}
	cephConfPath := effectiveCephConfPath()
	cephKeyringPath := effectiveCephKeyringPath()

	fi, err := os.Stat(cephConfPath)
	if err != nil {
		return fmt.Errorf("ceph conf file: %w", err)
	}
	if fi.IsDir() {
		return fmt.Errorf("ceph conf file: ディレクトリは指定できません")
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("ceph conf file: 通常ファイルを指定してください")
	}
	f, err := os.Open(cephConfPath)
	if err != nil {
		return fmt.Errorf("ceph conf file: %w", err)
	}
	_ = f.Close()

	fi, err = os.Stat(cephKeyringPath)
	if err != nil {
		return fmt.Errorf("ceph keyring file: %w", err)
	}
	if fi.IsDir() {
		return fmt.Errorf("ceph keyring file: ディレクトリは指定できません")
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("ceph keyring file: 通常ファイルを指定してください")
	}
	f, err = os.Open(cephKeyringPath)
	if err != nil {
		return fmt.Errorf("ceph keyring file: %w", err)
	}
	_ = f.Close()
	return nil
}
