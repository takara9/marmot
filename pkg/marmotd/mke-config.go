package marmotd

import (
	"encoding/json"
	"os"
)

// DefaultMKEConfigPath は MKS (Marmot Kubernetes Engine) の既定設定ファイルパスです。
const DefaultMKEConfigPath = "/etc/marmot/mke.json"

// MKEConfig は /etc/marmot/mke.json で設定可能な MKS (Marmot Kubernetes Engine) の
// 初期設定情報を保持します。KubernetesEngineコントローラーが、クラスタ構築時の
// デフォルトバージョン等として参照します。
type MKEConfig struct {
	// デプロイする Kubernetes のバージョン
	// 例: "v1.36.2"
	KubernetesVersion string `json:"kubernetes_version"`

	// containerd のバージョン
	// 例: "2.3.1"
	ContainerdVersion string `json:"containerd_version"`

	// クラスタ専用 etcd のバージョン
	// 例: "3.6.8"
	EtcdVersion string `json:"etcd_version"`

	// CNI プラグインのバージョン
	// 例: "0.4.0"
	CNIVersion string `json:"cni_version"`

	// 既定で使用する CNI の種類
	// 例: "bridge"
	DefaultCNIType string `json:"default_cni_type"`

	// runc のバージョン
	// 例: "1.4.0"
	RuncVersion string `json:"runc_version"`
}

// defaultMKEConfig はコンフィグファイルが存在しない場合や、一部フィールドが
// 指定されていない場合に使用されるデフォルト値を返します。
func defaultMKEConfig() *MKEConfig {
	return &MKEConfig{
		KubernetesVersion: "v1.36.2",
		ContainerdVersion: "2.3.1",
		EtcdVersion:       "3.6.8",
		CNIVersion:        "0.4.0",
		DefaultCNIType:    "bridge",
		RuncVersion:       "1.4.0",
	}
}

// LoadMKEConfig は path で指定された JSON ファイルを読み込み MKEConfig を返します。
// ファイルが存在しない場合はデフォルト値を持つ設定を返します。
// ファイルが存在するが一部フィールドが省略されている場合は、デフォルト値で補完されます。
func LoadMKEConfig(path string) (*MKEConfig, error) {
	cfg := defaultMKEConfig()

	f, err := os.Open(path)
	if os.IsNotExist(err) {
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
	return cfg, nil
}
