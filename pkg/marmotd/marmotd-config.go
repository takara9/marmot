package marmotd

import (
	"encoding/json"
	"os"
)

const DefaultConfigPath = "/etc/marmot/marmotd.json"

// MarmotdConfig は /etc/marmot/marmotd.json で設定可能なパラメータを保持します。
type MarmotdConfig struct {
	// marmot-API サーバーのバインドアドレスとポート番号
	// 例: "0.0.0.0:8750"
	APIListenAddr string `json:"api_listen_addr"`

	// internal-DNS サーバーのバインドアドレスとポート番号
	// 例: "127.0.0.1:53"
	DNSListenAddr string `json:"dns_listen_addr"`

	// DNS 外部参照先アドレス (フォワーダー)
	// 例: "8.8.8.8:53"
	DNSUpstream string `json:"dns_upstream"`

	// コントローラーが DeletionTimestamp を検知してから実際に削除処理を
	// 開始するまでの待機秒数
	DeletionDelaySeconds int `json:"deletion_delay_seconds"`
}

// defaultConfig はコンフィグファイルが存在しない場合や一部フィールドが
// 指定されていない場合に使用されるデフォルト値を返します。
func defaultConfig() *MarmotdConfig {
	return &MarmotdConfig{
		APIListenAddr:        "0.0.0.0:8750",
		DNSListenAddr:        "127.0.0.1:53",
		DNSUpstream:          "8.8.8.8:53",
		DeletionDelaySeconds: 10,
	}
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

	return cfg, nil
}
