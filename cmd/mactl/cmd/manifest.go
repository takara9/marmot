package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
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
	ManifestTypeServer     ManifestType = "Server"
	ManifestTypeImage      ManifestType = "Image"
	ManifestTypeVolume     ManifestType = "Volume"
	ManifestTypeNetwork    ManifestType = "VirtualNetwork"
	ManifestTypeGateway    ManifestType = "Gateway"
	ManifestTypeLoadBalancer ManifestType = "LoadBalancer"
	ManifestTypeVpnGateway ManifestType = "VpnGateway"
	ManifestTypeUnknown    ManifestType = "Unknown"
)

// Manifest マニフェストベース構造
type Manifest struct {
	ApiVersion string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                 `json:"kind" yaml:"kind"`
	Metadata   map[string]interface{} `json:"metadata" yaml:"metadata"`
	Spec       map[string]interface{} `json:"spec" yaml:"spec"`
}

// LoadManifest ファイル、URL、または stdin から最初のマニフェストを読み込む
func LoadManifest(source string) (map[string]interface{}, error) {
	manifests, err := LoadManifests(source)
	if err != nil {
		return nil, err
	}
	return manifests[0], nil
}

// LoadManifests ファイル、URL、または stdin から複数マニフェストを読み込む
func LoadManifests(source string) ([]map[string]interface{}, error) {
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

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	manifests := make([]map[string]interface{}, 0)
	for {
		var result map[string]interface{}
		err := decoder.Decode(&result)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}
		if len(result) == 0 {
			continue
		}
		manifests = append(manifests, result)
	}

	if len(manifests) == 0 {
		return nil, fmt.Errorf("no manifests found")
	}

	return manifests, nil
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
	case "gw", "gateways":
		return "gateway"
	case "lb", "lbs", "load-balancer", "load-balancers", "loadbalancer", "loadbalancers":
		return "loadbalancer"
	case "vpn-gateway", "vpn-gateways", "vpngateway", "vpngateways", "vpngw":
		return "vpngateway"
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
	case "gateway":
		return ManifestTypeGateway
	case "loadbalancer":
		return ManifestTypeLoadBalancer
	case "vpngateway":
		return ManifestTypeVpnGateway
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
	case "gateway":
		return "Gateway"
	case "loadbalancer":
		return "LoadBalancer"
	case "vpngateway":
		return "VpnGateway"
	default:
		return ""
	}
}

// ResolveResourceNameForManifest は manifest とコマンド引数から処理対象の resource 名を決める。
func ResolveResourceNameForManifest(manifest map[string]interface{}, args []string) (string, error) {
	kind := ""
	if k, ok := manifest["kind"].(string); ok {
		kind = k
	}

	var resourceName string
	if len(args) > 0 {
		resourceName = normalizeResourceName(args[0])
	} else {
		switch GetManifestType(kind) {
		case ManifestTypeServer:
			resourceName = "server"
		case ManifestTypeImage:
			resourceName = "image"
		case ManifestTypeVolume:
			resourceName = "volume"
		case ManifestTypeNetwork:
			resourceName = "network"
		case ManifestTypeGateway:
			resourceName = "gateway"
		case ManifestTypeLoadBalancer:
			resourceName = "loadbalancer"
		case ManifestTypeVpnGateway:
			resourceName = "vpngateway"
		default:
			return "", fmt.Errorf("failed to infer resource type from kind %q", kind)
		}
	}

	expectedKind := GetKindFromResourceName(resourceName)
	if kind != "" && kind != expectedKind {
		return "", fmt.Errorf("manifest kind %q does not match resource type %q", kind, resourceName)
	}

	return resourceName, nil
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

// ManifestToGateway マニフェストを Gateway 構造体に変換
func ManifestToGateway(manifest map[string]interface{}) (*api.Gateway, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var gateway api.Gateway
	if err := json.Unmarshal(data, &gateway); err != nil {
		return nil, fmt.Errorf("failed to parse as Gateway: %w", err)
	}

	return &gateway, nil
}

// ManifestToLoadBalancer マニフェストを LoadBalancer 構造体に変換
func ManifestToLoadBalancer(manifest map[string]interface{}) (*api.LoadBalancer, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var loadBalancer api.LoadBalancer
	if err := json.Unmarshal(data, &loadBalancer); err != nil {
		return nil, fmt.Errorf("failed to parse as LoadBalancer: %w", err)
	}

	return &loadBalancer, nil
}

// ManifestToVpnGateway マニフェストを VpnGateway 構造体に変換
func ManifestToVpnGateway(manifest map[string]interface{}) (*api.VpnGateway, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	var vpnGateway api.VpnGateway
	if err := json.Unmarshal(data, &vpnGateway); err != nil {
		return nil, fmt.Errorf("failed to parse as VpnGateway: %w", err)
	}

	return &vpnGateway, nil
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
