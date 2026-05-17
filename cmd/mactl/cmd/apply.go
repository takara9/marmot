package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var applyCmd = &cobra.Command{
	Use:   "apply [RESOURCE]",
	Short: "Create or update a resource from a file or stdin",
	Long:  `Apply a resource (server/srv, image/img, volume/vol, network/net) from a manifest file or stdin. Creates if not exists, updates if exists. If RESOURCE is omitted, it is inferred from manifest kind.`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// マニフェストファイルが指定されていない場合はエラー
		if manifestFile == "" {
			return fmt.Errorf("flag -f is required for apply command")
		}

		// マニフェストを読み込む
		manifest, err := LoadManifest(manifestFile)
		if err != nil {
			return fmt.Errorf("failed to load manifest: %w", err)
		}

		// マニフェストの kind を取得
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
			default:
				return fmt.Errorf("failed to infer resource type from kind %q", kind)
			}
		}

		expectedKind := GetKindFromResourceName(resourceName)
		if kind != "" && kind != expectedKind {
			return fmt.Errorf("manifest kind %q does not match resource type %q", kind, resourceName)
		}

		// リソースタイプに応じて処理を分岐
		switch strings.ToLower(resourceName) {
		case "server":
			return applyServer(manifest)
		case "image":
			return applyImage(manifest)
		case "volume":
			return applyVolume(manifest)
		case "network":
			return applyNetwork(manifest)
		default:
			return fmt.Errorf("unknown resource type: %s", resourceName)
		}
	},
}

func applyServer(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	// Server オブジェクトに変換
	server, err := ManifestToServer(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to server: %w", err)
	}

	// 必須フィールドの確認
	if server.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if server.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(server.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}

	// storage[].spec.type/kind の既定値を補完
	ApplyServerDefaults(server)

	// サーバーが既に存在するかチェック
	exists := false
	var existingId string
	list, _, err := m.GetServers()
	if err == nil {
		var servers []api.Server
		json.Unmarshal(list, &servers)
		for _, s := range servers {
			if s.Metadata.Name == server.Metadata.Name {
				exists = true
				existingId = api.ServerID(s)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		// 更新
		api.SetServerID(server, existingId)
		byteBody, _, err = m.UpdateServerById(existingId, *server)
		if err != nil {
			return fmt.Errorf("failed to update server: %w", err)
		}
	} else {
		// 作成
		byteBody, _, err = m.CreateServer(*server)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}
	}

	// レスポンス処理
	return processApplyResponse(byteBody, exists)
}

func applyImage(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	// Image オブジェクトに変換
	image, err := ManifestToImage(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to image: %w", err)
	}

	// 必須フィールドの確認
	if image.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if image.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(image.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if image.Spec.SourceUrl == nil || strings.TrimSpace(*image.Spec.SourceUrl) == "" {
		return fmt.Errorf("spec.sourceUrl is required")
	}

	// イメージが既に存在するかチェック
	exists := false
	var existingId string
	list, _, err := m.GetImages()
	if err == nil {
		var images []api.Image
		json.Unmarshal(list, &images)
		for _, img := range images {
			if img.Metadata.Name == image.Metadata.Name {
				exists = true
				existingId = img.Metadata.Id
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		// 更新
		image.Metadata.Id = existingId
		byteBody, _, err = m.UpdateImageById(existingId, *image)
		if err != nil {
			return fmt.Errorf("failed to update image: %w", err)
		}
	} else {
		// 作成
		byteBody, _, err = m.CreateImage(*image)
		if err != nil {
			return fmt.Errorf("failed to create image: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func applyVolume(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	// Volume オブジェクトに変換
	volume, err := ManifestToVolume(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to volume: %w", err)
	}

	// 必須フィールドの確認
	if volume.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if volume.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(volume.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if volume.Spec.Type == nil || strings.TrimSpace(*volume.Spec.Type) == "" {
		return fmt.Errorf("spec.type is required")
	}
	if volume.Spec.Kind == nil || strings.TrimSpace(*volume.Spec.Kind) == "" {
		return fmt.Errorf("spec.kind is required")
	}

	// ボリュームが既に存在するかチェック
	exists := false
	var existingId string
	list, _, err := m.ListVolumes()
	if err == nil {
		var volumes []api.Volume
		json.Unmarshal(list, &volumes)
		for _, vol := range volumes {
			if vol.Metadata.Name == volume.Metadata.Name {
				exists = true
				existingId = api.VolumeID(vol)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		// 更新
		api.SetVolumeID(volume, existingId)
		byteBody, _, err = m.UpdateVolumeById(existingId, *volume)
		if err != nil {
			return fmt.Errorf("failed to update volume: %w", err)
		}
	} else {
		// 作成
		byteBody, _, err = m.CreateVolume(*volume)
		if err != nil {
			return fmt.Errorf("failed to create volume: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func applyNetwork(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	// VirtualNetwork オブジェクトに変換
	network, err := ManifestToVirtualNetwork(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to network: %w", err)
	}

	// 必須フィールドの確認
	if network.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if network.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(network.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}

	// ネットワークが既に存在するかチェック
	exists := false
	var existingId string
	list, _, err := m.GetVirtualNetworks()
	if err == nil {
		var networks []api.VirtualNetwork
		json.Unmarshal(list, &networks)
		for _, net := range networks {
			if net.Metadata.Name == network.Metadata.Name {
				exists = true
				existingId = api.VirtualNetworkID(net)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		// 更新
		api.SetVirtualNetworkID(network, existingId)
		byteBody, _, err = m.UpdateVirtualNetworkById(existingId, *network)
		if err != nil {
			return fmt.Errorf("failed to update network: %w", err)
		}
	} else {
		// 作成
		byteBody, _, err = m.CreateVirtualNetwork(*network)
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func processApplyResponse(byteBody []byte, updated bool) error {
	switch outputStyle {
	case "text":
		var data any
		if err := json.Unmarshal(byteBody, &data); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		serveMap := data.(map[string]any)
		if updated {
			fmt.Printf("リソースが更新されました。ID: %v\n", serveMap["id"])
		} else {
			fmt.Printf("リソースが作成されました。ID: %v\n", serveMap["id"])
		}
		return nil

	case "json":
		fmt.Println(string(byteBody))
		return nil

	case "yaml":
		var data interface{}
		if err := json.Unmarshal(byteBody, &data); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal to YAML: %w", err)
		}
		fmt.Println(string(yamlBytes))
		return nil

	default:
		return fmt.Errorf("output style must be text/json/yaml")
	}
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().StringVarP(&manifestFile, "file", "f", "", "Manifest file, URL, or - for stdin")
	applyCmd.MarkFlagRequired("file")
}
