package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
)

var delManifestFile string

var delCmd = &cobra.Command{
	Use:   "del [RESOURCE NAME]",
	Short: "Delete a resource",
	Long:  `Delete a resource (server/srv, image/img, volume/vol, network/net, gateway/gw, vpngateway/vpngw) with NAME specified. With -f, process manifest(s) and delete by metadata.name for each document.`,
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(delManifestFile) != "" {
			if len(args) > 1 {
				return fmt.Errorf("with -f, del accepts at most one optional RESOURCE argument")
			}

			manifests, err := LoadManifests(delManifestFile)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}

			for index, manifest := range manifests {
				resourceName, err := ResolveResourceNameForManifest(manifest, args)
				if err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}

				objectName := strings.TrimSpace(ExtractMetadataName(manifest))
				if objectName == "" {
					return fmt.Errorf("manifest %d: metadata.name is required", index+1)
				}

				if err := deleteResourceByTypeAndName(resourceName, objectName); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			}

			return nil
		}

		if len(args) != 2 {
			return fmt.Errorf("del requires RESOURCE and NAME unless -f is specified")
		}

		resourceName := normalizeResourceName(args[0])
		objectName := args[1]

		return deleteResourceByTypeAndName(resourceName, objectName)
	},
}


func deleteResourceByTypeAndName(resourceName string, objectName string) error {
	// リソースタイプに応じて処理を分岐
	switch strings.ToLower(resourceName) {
	case "server":
		return deleteServer(objectName)
	case "image":
		return deleteImage(objectName)
	case "volume":
		return deleteVolume(objectName)
	case "network":
		return deleteNetwork(objectName)
	case "gateway":
		return deleteGateway(objectName)
	case "vpngateway":
		return deleteVpnGateway(objectName)
	default:
		return fmt.Errorf("unknown resource type: %s", resourceName)
	}
}

func deleteServer(name string) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	list, _, err := m.GetServers()
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	var servers []api.Server
	if err := json.Unmarshal(list, &servers); err != nil {
		return fmt.Errorf("failed to parse servers: %w", err)
	}

	// 一致するサーバーを検索
	var found *api.Server
	for i := range servers {
		if servers[i].Metadata.Name == name {
			found = &servers[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("server %q not found", name)
	}

	// サーバーを削除
	_, _, err = m.DeleteServerById(api.ServerID(*found))
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	fmt.Printf("server %q deletion requested (accepted)\n", name)
	return nil
}

func deleteImage(name string) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	list, _, err := m.GetImages()
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	var images []api.Image
	if err := json.Unmarshal(list, &images); err != nil {
		return fmt.Errorf("failed to parse images: %w", err)
	}

	// 一致するイメージを検索
	var found *api.Image
	for i := range images {
		if images[i].Metadata.Name == name {
			found = &images[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("image %q not found", name)
	}

	// イメージを削除
	_, _, err = m.DeleteImageById(found.Metadata.Id)
	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	fmt.Printf("image %q deletion requested (accepted)\n", name)
	return nil
}

func deleteVolume(name string) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	list, _, err := m.ListVolumes()
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}

	var volumes []api.Volume
	if err := json.Unmarshal(list, &volumes); err != nil {
		return fmt.Errorf("failed to parse volumes: %w", err)
	}

	// 一致するボリュームを検索
	var found *api.Volume
	for i := range volumes {
		if volumes[i].Metadata.Name == name {
			found = &volumes[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("volume %q not found", name)
	}

	// ボリュームを削除
	_, _, err = m.DeleteVolumeById(api.VolumeID(*found))
	if err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	fmt.Printf("volume %q deletion requested (accepted)\n", name)
	return nil
}

func deleteNetwork(name string) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	list, _, err := m.GetVirtualNetworks()
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	var networks []api.VirtualNetwork
	if err := json.Unmarshal(list, &networks); err != nil {
		return fmt.Errorf("failed to parse networks: %w", err)
	}

	// 一致するネットワークをすべて検索し、削除要求を送る。
	// 同名の head/follower エントリーが複数ある場合に、先頭1件だけ削除要求される問題を防ぐ。
	matches := make([]api.VirtualNetwork, 0)
	for _, net := range networks {
		if net.Metadata.Name == name {
			matches = append(matches, net)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("network %q not found", name)
	}

	for _, net := range matches {
		if _, _, err = m.DeleteVirtualNetworkById(api.VirtualNetworkID(net)); err != nil {
			return fmt.Errorf("failed to delete network %q(id=%s): %w", name, api.VirtualNetworkID(net), err)
		}
	}

	fmt.Printf("network %q deletion requested (accepted) for %d object(s)\n", name, len(matches))
	return nil
}

func deleteGateway(name string) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	list, _, err := m.GetGateways()
	if err != nil {
		return fmt.Errorf("failed to list gateways: %w", err)
	}

	var gateways []api.Gateway
	if err := json.Unmarshal(list, &gateways); err != nil {
		return fmt.Errorf("failed to parse gateways: %w", err)
	}

	matches := make([]api.Gateway, 0)
	for _, g := range gateways {
		if g.Metadata.Name == name {
			matches = append(matches, g)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("gateway %q not found", name)
	}
	if len(matches) > 1 {
		return fmt.Errorf("multiple gateways found with name %q; please delete by id via API", name)
	}

	_, _, err = m.DeleteGatewayById(api.GatewayID(matches[0]))
	if err != nil {
		return fmt.Errorf("failed to delete gateway: %w", err)
	}

	fmt.Printf("gateway %q deletion requested (accepted)\n", name)
	return nil
}

func deleteVpnGateway(name string) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	list, _, err := m.GetVpnGateways()
	if err != nil {
		return fmt.Errorf("failed to list vpn gateways: %w", err)
	}

	var items []api.VpnGateway
	if err := json.Unmarshal(list, &items); err != nil {
		return fmt.Errorf("failed to parse vpn gateways: %w", err)
	}

	matches := make([]api.VpnGateway, 0)
	for _, g := range items {
		if g.Metadata.Name == name {
			matches = append(matches, g)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("vpn gateway %q not found", name)
	}
	if len(matches) > 1 {
		return fmt.Errorf("multiple vpn gateways found with name %q; please delete by id via API", name)
	}

	_, _, err = m.DeleteVpnGatewayById(api.VpnGatewayID(matches[0]))
	if err != nil {
		return fmt.Errorf("failed to delete vpn gateway: %w", err)
	}

	fmt.Printf("vpn gateway %q deletion requested (accepted)\n", name)
	return nil
}

func init() {
	rootCmd.AddCommand(delCmd)
	delCmd.Flags().StringVarP(&delManifestFile, "file", "f", "", "Manifest file, URL, or - for stdin")
}
