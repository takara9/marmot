package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"go.yaml.in/yaml/v3"
)

// ManifestType リソースタイプの定義
type ManifestType string

const (
	ManifestTypeServer  ManifestType = "Server"
	ManifestTypeImage   ManifestType = "Image"
	ManifestTypeVolume  ManifestType = "Volume"
	ManifestTypeNetwork ManifestType = "VirtualNetwork"
	ManifestTypeUnknown ManifestType = "Unknown"
)

// Manifest マニフェストベース構造
type Manifest struct {
	ApiVersion string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                 `json:"kind" yaml:"kind"`
	Metadata   map[string]interface{} `json:"metadata" yaml:"metadata"`
	Spec       map[string]interface{} `json:"spec" yaml:"spec"`
}

// LoadManifest ファイル、URL、または stdin からマニフェストを読み込む
func LoadManifest(source string) (map[string]interface{}, error) {
	var data []byte
	var err error

	if source == "-" || source == "" {
		// stdin から読み込み
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
	} else if isURL(source) {
		// URL から読み込み
		resp, err := http.Get(source)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch URL: %w", err)
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
	} else {
		// ファイルから読み込み
		data, err = os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	// YAML または JSON として解析
	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		// JSON として解析を試みる
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}
	}

	return result, nil
}

// isURL URL かどうかを判定
func isURL(source string) bool {
	u, err := url.Parse(source)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// normalizeResourceName リソース名を正規化（短縮形を長いフォームに変換）
func normalizeResourceName(resource string) string {
	switch strings.ToLower(resource) {
	case "srv", "servers":
		return "server"
	case "img", "images":
		return "image"
	case "vol", "volumes":
		return "volume"
	case "net", "networks":
		return "network"
	default:
		return strings.ToLower(resource)
	}
}

// GetManifestType Kind から ManifestType を取得
func GetManifestType(kind string) ManifestType {
	switch strings.ToLower(kind) {
	case "server":
		return ManifestTypeServer
	case "image":
		return ManifestTypeImage
	case "volume":
		return ManifestTypeVolume
	case "virtualnetwork":
		return ManifestTypeNetwork
	default:
		return ManifestTypeUnknown
	}
}

// GetKindFromResourceName リソース名から Kind を取得
func GetKindFromResourceName(resource string) string {
	normalized := normalizeResourceName(resource)
	switch normalized {
	case "server":
		return "Server"
	case "image":
		return "Image"
	case "volume":
		return "Volume"
	case "network":
		return "VirtualNetwork"
	default:
		return ""
	}
}

// ManifestToServer マニフェストを Server 構造体に変換
func ManifestToServer(manifest map[string]interface{}) (*api.Server, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var server api.Server
	if err := json.Unmarshal(data, &server); err != nil {
		return nil, fmt.Errorf("failed to parse as Server: %w", err)
	}

	return &server, nil
}

// ApplyServerDefaults は server.spec.storage 配下の既定値を補完する。
// storage[].spec.type が未指定なら qcow2、storage[].spec.kind が未指定なら data を設定する。
func ApplyServerDefaults(server *api.Server) {
	if server == nil || server.Spec.Storage == nil {
		return
	}

	for i := range *server.Spec.Storage {
		vol := &(*server.Spec.Storage)[i]
		if vol.Spec.Type == nil || strings.TrimSpace(*vol.Spec.Type) == "" {
			vol.Spec.Type = util.StringPtr("qcow2")
		}
		if vol.Spec.Kind == nil || strings.TrimSpace(*vol.Spec.Kind) == "" {
			vol.Spec.Kind = util.StringPtr("data")
		}
	}
}

// ManifestToImage マニフェストを Image 構造体に変換
func ManifestToImage(manifest map[string]interface{}) (*api.Image, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var image api.Image
	if err := json.Unmarshal(data, &image); err != nil {
		return nil, fmt.Errorf("failed to parse as Image: %w", err)
	}

	return &image, nil
}

// ManifestToVolume マニフェストを Volume 構造体に変換
func ManifestToVolume(manifest map[string]interface{}) (*api.Volume, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var volume api.Volume
	if err := json.Unmarshal(data, &volume); err != nil {
		return nil, fmt.Errorf("failed to parse as Volume: %w", err)
	}

	return &volume, nil
}

// ManifestToVirtualNetwork マニフェストを VirtualNetwork 構造体に変換
func ManifestToVirtualNetwork(manifest map[string]interface{}) (*api.VirtualNetwork, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var network api.VirtualNetwork
	if err := json.Unmarshal(data, &network); err != nil {
		return nil, fmt.Errorf("failed to parse as VirtualNetwork: %w", err)
	}

	return &network, nil
}

// ExtractMetadataName マニフェストからメタデータ名を抽出
func ExtractMetadataName(manifest map[string]interface{}) string {
	if metadata, ok := manifest["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			return name
		}
	}
	return ""
}

// ExtractLabels マニフェストからラベルを抽出
func ExtractLabels(manifest map[string]interface{}) map[string]interface{} {
	if metadata, ok := manifest["metadata"].(map[string]interface{}); ok {
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			return labels
		}
	}
	return nil
}

// MatchesLabel ラベルがフィルターに一致するかを確認
func MatchesLabel(labels map[string]interface{}, labelFilter string) bool {
	if labelFilter == "" || labels == nil {
		return true
	}

	// labelFilter 形式: key=value または key!=value
	parts := strings.SplitN(labelFilter, "=", 2)
	if len(parts) != 2 {
		return false
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	// != 形式の場合
	if strings.HasPrefix(key, "!") {
		key = key[1:]
		if labelValue, ok := labels[key]; ok {
			return labelValue != value
		}
		return true
	}

	// 通常の = 形式
	if labelValue, ok := labels[key]; ok {
		return fmt.Sprintf("%v", labelValue) == value
	}
	return false
}
