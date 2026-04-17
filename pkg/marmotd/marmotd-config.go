package marmotd

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/takara9/marmot/pkg/db"
)

const DefaultConfigPath = "/etc/marmot/marmotd.json"

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

	// OS ボリューム用の LVM Volume Group 名
	// 例: "vg1"
	OSVolumeGroup string `json:"os_volume_group"`

	// DATA ボリューム用の LVM Volume Group 名
	// 例: "vg2"
	DataVolumeGroup string `json:"data_volume_group"`

	// コントローラーが DeletionTimestamp を検知してから実際に削除処理を
	// 開始するまでの待機秒数
	DeletionDelaySeconds int `json:"deletion_delay_seconds"`
}

var runtimeConfigState = struct {
	mu  sync.RWMutex
	cfg *MarmotdConfig
}{
	cfg: defaultConfig(),
}

// defaultConfig はコンフィグファイルが存在しない場合や一部フィールドが
// 指定されていない場合に使用されるデフォルト値を返します。
func defaultConfig() *MarmotdConfig {
	return &MarmotdConfig{
		NodeName:             "hv1",
		EtcdURL:              "http://127.0.0.1:2379",
		APIListenAddr:        "0.0.0.0:8750",
		DNSListenAddr:        "127.0.0.1:53",
		DNSUpstream:          "8.8.8.8:53",
		OSVolumeGroup:        db.DefaultOSVolumeGroup,
		DataVolumeGroup:      db.DefaultDataVolumeGroup,
		DeletionDelaySeconds: 10,
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
		normalized.DNSListenAddr = defaults.DNSListenAddr
	}
	if strings.TrimSpace(normalized.DNSUpstream) == "" {
		normalized.DNSUpstream = defaults.DNSUpstream
	}
	if strings.TrimSpace(normalized.OSVolumeGroup) == "" {
		normalized.OSVolumeGroup = db.DefaultOSVolumeGroup
	}
	if strings.TrimSpace(normalized.DataVolumeGroup) == "" {
		normalized.DataVolumeGroup = db.DefaultDataVolumeGroup
	}
	if normalized.DeletionDelaySeconds <= 0 {
		normalized.DeletionDelaySeconds = defaults.DeletionDelaySeconds
	}
	return normalized
}

func SetRuntimeConfig(cfg *MarmotdConfig) {
	normalized := normalizeConfig(cfg)

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
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, err
	}

	return normalizeConfig(cfg), nil
}
