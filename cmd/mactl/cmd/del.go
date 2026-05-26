package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
)

var delCmd = &cobra.Command{
	Use:   "del RESOURCE NAME",
	Short: "Delete a resource",
	Long:  `Delete a resource (server/srv, image/img, volume/vol, network/net, gateway/gw) with NAME specified.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		resourceName := normalizeResourceName(args[0])
		objectName := args[1]

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
		default:
			return fmt.Errorf("unknown resource type: %s", resourceName)
		}
	},
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

	fmt.Printf("server %q deleted successfully\n", name)
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

	fmt.Printf("image %q deleted successfully\n", name)
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

	fmt.Printf("volume %q deleted successfully\n", name)
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

	// 一致するネットワークを検索
	var found *api.VirtualNetwork
	for i := range networks {
		if networks[i].Metadata.Name == name {
			found = &networks[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("network %q not found", name)
	}

	// ネットワークを削除
	_, _, err = m.DeleteVirtualNetworkById(api.VirtualNetworkID(*found))
	if err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}

	fmt.Printf("network %q deleted successfully\n", name)
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

	fmt.Printf("gateway %q deleted successfully\n", name)
	return nil
}

func init() {
	rootCmd.AddCommand(delCmd)
}
