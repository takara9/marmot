package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var describeCmd = &cobra.Command{
	Use:   "describe RESOURCE NAME",
	Short: "Show detailed information about a resource",
	Long:  `Describe a resource (server/srv, image/img, volume/vol, network/net) with NAME specified. Shows formatted text output.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		resourceName := args[0]
		objectName := args[1]
		resourceName = normalizeResourceName(resourceName)

		// リソースタイプに応じて処理を分岐
		switch strings.ToLower(resourceName) {
		case "server":
			return describeServer(objectName)
		case "image":
			return describeImage(objectName)
		case "volume":
			return describeVolume(objectName)
		case "network":
			return describeNetwork(objectName)
		default:
			return fmt.Errorf("unknown resource type: %s", resourceName)
		}
	},
}

func describeServer(name string) error {
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

	return describeResource(found)
}

func describeImage(name string) error {
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

	return describeResource(found)
}

func describeVolume(name string) error {
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

	return describeResource(found)
}

func describeNetwork(name string) error {
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

	return describeResource(found)
}

func describeResource(resource interface{}) error {
	switch outputStyle {
	case "text":
		// JSON -> YAML -> テキストフォーマット
		data, err := json.MarshalIndent(resource, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal: %w", err)
		}
		fmt.Println(string(data))
		return nil

	case "json":
		data, err := json.MarshalIndent(resource, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal: %w", err)
		}
		fmt.Println(string(data))
		return nil

	case "yaml":
		data, err := json.Marshal(resource)
		if err != nil {
			return fmt.Errorf("failed to marshal: %w", err)
		}
		var obj interface{}
		json.Unmarshal(data, &obj)
		yamlBytes, err := yaml.Marshal(obj)
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
	rootCmd.AddCommand(describeCmd)
}
