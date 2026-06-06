package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var manifestFile string

var createCmd = &cobra.Command{
	Use:   "create [RESOURCE]",
	Short: "Create a resource from a file or stdin",
	Long:  `Create a resource (server/srv, image/img, volume/vol, network/net, gateway/gw, applicationloadbalancer/alb, vpngateway/vpngw) from a manifest file or stdin. If RESOURCE is omitted, it is inferred from manifest kind.`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// マニフェストファイルが指定されていない場合はエラー
		if manifestFile == "" {
			return fmt.Errorf("flag -f is required for create command")
		}

		// マニフェストを読み込む
		manifests, err := LoadManifests(manifestFile)
		if err != nil {
			return fmt.Errorf("failed to load manifest: %w", err)
		}

		for index, manifest := range manifests {
			resourceName, err := ResolveResourceNameForManifest(manifest, args)
			if err != nil {
				return fmt.Errorf("manifest %d: %w", index+1, err)
			}

			switch strings.ToLower(resourceName) {
			case "server":
				if err := createServer(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "image":
				if err := createImage(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "volume":
				if err := createVolume(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "network":
				if err := createNetwork(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "gateway":
				if err := createGateway(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "applicationloadbalancer":
				if err := createLoadBalancer(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "vpngateway":
				if err := createVpnGateway(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			default:
				return fmt.Errorf("manifest %d: unknown resource type: %s", index+1, resourceName)
			}
		}

		return nil
	},
}

func createServer(manifest map[string]interface{}) error {
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
	list, _, err := m.GetServers()
	if err == nil {
		var servers []api.Server
		json.Unmarshal(list, &servers)
		for _, s := range servers {
			if s.Metadata.Name == server.Metadata.Name {
				return fmt.Errorf("server %q already exists", server.Metadata.Name)
			}
		}
	}

	// サーバーを作成
	byteBody, _, err := m.CreateServer(*server)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// レスポンス処理
	return processCreateResponse(byteBody)
}

func createImage(manifest map[string]interface{}) error {
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
	list, _, err := m.GetImages()
	if err == nil {
		var images []api.Image
		json.Unmarshal(list, &images)
		for _, img := range images {
			if img.Metadata.Name == image.Metadata.Name {
				return fmt.Errorf("image %q already exists", image.Metadata.Name)
			}
		}
	}

	// イメージを作成
	byteBody, _, err := m.CreateImage(*image)
	if err != nil {
		return fmt.Errorf("failed to create image: %w", err)
	}

	return processCreateResponse(byteBody)
}

func createVolume(manifest map[string]interface{}) error {
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
	list, _, err := m.ListVolumes()
	if err == nil {
		var volumes []api.Volume
		json.Unmarshal(list, &volumes)
		for _, vol := range volumes {
			if vol.Metadata.Name == volume.Metadata.Name {
				return fmt.Errorf("volume %q already exists", volume.Metadata.Name)
			}
		}
	}

	// ボリュームを作成
	byteBody, _, err := m.CreateVolume(*volume)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}

	return processCreateResponse(byteBody)
}

func createNetwork(manifest map[string]interface{}) error {
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
	list, _, err := m.GetVirtualNetworks()
	if err == nil {
		var networks []api.VirtualNetwork
		json.Unmarshal(list, &networks)
		for _, net := range networks {
			if net.Metadata.Name == network.Metadata.Name {
				return fmt.Errorf("network %q already exists", network.Metadata.Name)
			}
		}
	}

	// ネットワークを作成
	byteBody, _, err := m.CreateVirtualNetwork(*network)
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	return processCreateResponse(byteBody)
}

func createGateway(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	gateway, err := ManifestToGateway(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to gateway: %w", err)
	}

	if gateway.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if gateway.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(gateway.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(gateway.Spec.InternalVirtualNetwork) == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	list, _, err := m.GetGateways()
	if err == nil {
		var gateways []api.Gateway
		json.Unmarshal(list, &gateways)
		for _, g := range gateways {
			if g.Metadata.Name != gateway.Metadata.Name {
				continue
			}
			if strings.TrimSpace(g.Spec.InternalVirtualNetwork) == strings.TrimSpace(gateway.Spec.InternalVirtualNetwork) {
				return fmt.Errorf("gateway %q already exists in internalVirtualNetwork %q", gateway.Metadata.Name, gateway.Spec.InternalVirtualNetwork)
			}
		}
	}

	byteBody, _, err := m.CreateGateway(*gateway)
	if err != nil {
		return fmt.Errorf("failed to create gateway: %w", err)
	}

	return processCreateResponse(byteBody)
}

func createVpnGateway(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	vpnGateway, err := ManifestToVpnGateway(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to vpn gateway: %w", err)
	}

	if vpnGateway.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if vpnGateway.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(vpnGateway.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(vpnGateway.Spec.InternalVirtualNetwork) == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	list, _, err := m.GetVpnGateways()
	if err == nil {
		var items []api.VpnGateway
		json.Unmarshal(list, &items)
		for _, g := range items {
			if g.Metadata.Name == vpnGateway.Metadata.Name && strings.TrimSpace(g.Spec.InternalVirtualNetwork) == strings.TrimSpace(vpnGateway.Spec.InternalVirtualNetwork) {
				return fmt.Errorf("vpn gateway %q already exists in internalVirtualNetwork %q", vpnGateway.Metadata.Name, vpnGateway.Spec.InternalVirtualNetwork)
			}
		}
	}

	byteBody, _, err := m.CreateVpnGateway(*vpnGateway)
	if err != nil {
		return fmt.Errorf("failed to create vpn gateway: %w", err)
	}

	return processCreateResponse(byteBody)
}

func createLoadBalancer(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	loadBalancer, err := ManifestToLoadBalancer(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to load balancer: %w", err)
	}

	if loadBalancer.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if loadBalancer.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(loadBalancer.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork) == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	list, _, err := m.GetLoadBalancers()
	if err == nil {
		var items []api.ApplicationLoadBalancer
		json.Unmarshal(list, &items)
		for _, lb := range items {
			if lb.Metadata.Name == loadBalancer.Metadata.Name && strings.TrimSpace(lb.Spec.InternalVirtualNetwork) == strings.TrimSpace(loadBalancer.Spec.InternalVirtualNetwork) {
				return fmt.Errorf("load balancer %q already exists in internalVirtualNetwork %q", loadBalancer.Metadata.Name, loadBalancer.Spec.InternalVirtualNetwork)
			}
		}
	}

	byteBody, _, err := m.CreateLoadBalancer(*loadBalancer)
	if err != nil {
		return fmt.Errorf("failed to create load balancer: %w", err)
	}

	return processCreateResponse(byteBody)
}

func processCreateResponse(byteBody []byte) error {
	switch outputStyle {
	case "text":
		var data any
		if err := json.Unmarshal(byteBody, &data); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		serveMap := data.(map[string]any)
		fmt.Printf("リソースの作成要求が受け入れられました。ID: %v\n", serveMap["id"])
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
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringVarP(&manifestFile, "file", "f", "", "Manifest file, URL, or - for stdin")
	createCmd.MarkFlagRequired("file")
}
