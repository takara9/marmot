package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
)

var getCmd = &cobra.Command{
	Use:   "get RESOURCE [NAME]",
	Short: "Get resource(s) of a specific type",
	Long:  `Get resource(s) (server/srv, image/img, volume/vol, network/net). If NAME is provided, show only that resource. Otherwise, list all resources.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resourceName := args[0]
		resourceName = normalizeResourceName(resourceName)

		// NAME が指定されているかチェック
		var resourceSpec string
		if len(args) > 1 {
			resourceSpec = args[1]
		}

		// リソースタイプに応じて処理を分岐
		switch strings.ToLower(resourceName) {
		case "server":
			return getServerResources(resourceSpec)
		case "image":
			return getImageResources(resourceSpec)
		case "volume":
			return getVolumeResources(resourceSpec)
		case "network":
			return getNetworkResources(resourceSpec)
		default:
			return fmt.Errorf("unknown resource type: %s", resourceName)
		}
	},
}

func getServerResources(name string) error {
	listFn := func() error {
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

		// NAME でフィルター
		if name != "" {
			servers = filterServersByName(servers, name)
			if len(servers) == 0 {
				fmt.Printf("no server found with name %q\n", name)
				return nil
			}
		}

		// ラベルでフィルター
		if labelSelector != "" {
			servers = filterServersByLabel(servers, labelSelector)
		}

		// 出力
		return outputServers(servers)
	}

	return runList(listFn)
}

func getImageResources(name string) error {
	listFn := func() error {
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

		// NAME でフィルター
		if name != "" {
			images = filterImagesByName(images, name)
			if len(images) == 0 {
				fmt.Printf("no image found with name %q\n", name)
				return nil
			}
		}

		// ラベルでフィルター
		if labelSelector != "" {
			images = filterImagesByLabel(images, labelSelector)
		}

		// 出力
		return outputImages(images)
	}

	return runList(listFn)
}

func getVolumeResources(name string) error {
	listFn := func() error {
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

		// NAME でフィルター
		if name != "" {
			volumes = filterVolumesByName(volumes, name)
			if len(volumes) == 0 {
				fmt.Printf("no volume found with name %q\n", name)
				return nil
			}
		}

		// ラベルでフィルター
		if labelSelector != "" {
			volumes = filterVolumesByLabel(volumes, labelSelector)
		}

		// 出力
		return outputVolumes(volumes)
	}

	return runList(listFn)
}

func getNetworkResources(name string) error {
	listFn := func() error {
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

		// NAME でフィルター
		if name != "" {
			networks = filterNetworksByName(networks, name)
			if len(networks) == 0 {
				fmt.Printf("no network found with name %q\n", name)
				return nil
			}
		}

		// ラベルでフィルター
		if labelSelector != "" {
			networks = filterNetworksByLabel(networks, labelSelector)
		}

		// 出力
		return outputNetworks(networks)
	}

	return runList(listFn)
}

// フィルター関数
func filterServersByName(servers []api.Server, name string) []api.Server {
	var result []api.Server
	for _, s := range servers {
		if s.Metadata.Name == name {
			result = append(result, s)
		}
	}
	return result
}

func filterServersByLabel(servers []api.Server, labelFilter string) []api.Server {
	var result []api.Server
	for _, s := range servers {
		if MatchesLabel(convertLabels(s.Metadata.Labels), labelFilter) {
			result = append(result, s)
		}
	}
	return result
}

func filterImagesByName(images []api.Image, name string) []api.Image {
	var result []api.Image
	for _, img := range images {
		if img.Metadata.Name == name {
			result = append(result, img)
		}
	}
	return result
}

func filterImagesByLabel(images []api.Image, labelFilter string) []api.Image {
	var result []api.Image
	for _, img := range images {
		if MatchesLabel(convertLabels(img.Metadata.Labels), labelFilter) {
			result = append(result, img)
		}
	}
	return result
}

func filterVolumesByName(volumes []api.Volume, name string) []api.Volume {
	var result []api.Volume
	for _, v := range volumes {
		if v.Metadata.Name == name {
			result = append(result, v)
		}
	}
	return result
}

func filterVolumesByLabel(volumes []api.Volume, labelFilter string) []api.Volume {
	var result []api.Volume
	for _, v := range volumes {
		if MatchesLabel(convertLabels(v.Metadata.Labels), labelFilter) {
			result = append(result, v)
		}
	}
	return result
}

func filterNetworksByName(networks []api.VirtualNetwork, name string) []api.VirtualNetwork {
	var result []api.VirtualNetwork
	for _, n := range networks {
		if n.Metadata.Name == name {
			result = append(result, n)
		}
	}
	return result
}

func filterNetworksByLabel(networks []api.VirtualNetwork, labelFilter string) []api.VirtualNetwork {
	var result []api.VirtualNetwork
	for _, n := range networks {
		if MatchesLabel(convertLabels(n.Metadata.Labels), labelFilter) {
			result = append(result, n)
		}
	}
	return result
}

// ヘルパー関数
func convertLabels(labels *map[string]interface{}) map[string]interface{} {
	if labels == nil {
		return make(map[string]interface{})
	}
	return *labels
}

// 出力関数
func outputServers(servers []api.Server) error {
	switch outputStyle {
	case "text":
		fmt.Println("NAME                APIVERSION  KIND    STATUS")
		fmt.Println("----                ----------  ----    ------")
		for _, s := range servers {
			status := ""
			if s.Status != nil && s.Status.Status != nil {
				status = *s.Status.Status
			}
			fmt.Printf("%-20s  %-10s  %-7s  %s\n", s.Metadata.Name, s.ApiVersion, s.Kind, status)
		}
		return nil

	case "json":
		data, _ := json.MarshalIndent(servers, "", "  ")
		fmt.Println(string(data))
		return nil

	case "yaml":
		data, _ := json.Marshal(servers)
		var servers interface{}
		json.Unmarshal(data, &servers)
		// YAML フォーマットは describe で実装済みのものを参照
		fmt.Println(string(data))
		return nil

	default:
		return fmt.Errorf("output style must be text/json/yaml")
	}
}

func outputImages(images []api.Image) error {
	switch outputStyle {
	case "text":
		fmt.Println("NAME                APIVERSION  KIND    SOURCE URL")
		fmt.Println("----                ----------  ----    ----------")
		for _, img := range images {
			sourceUrl := ""
			if img.Spec.SourceUrl != nil {
				sourceUrl = *img.Spec.SourceUrl
			}
			fmt.Printf("%-20s  %-10s  %-7s  %s\n", img.Metadata.Name, img.ApiVersion, img.Kind, sourceUrl)
		}
		return nil

	case "json":
		data, _ := json.MarshalIndent(images, "", "  ")
		fmt.Println(string(data))
		return nil

	case "yaml":
		data, _ := json.Marshal(images)
		fmt.Println(string(data))
		return nil

	default:
		return fmt.Errorf("output style must be text/json/yaml")
	}
}

func outputVolumes(volumes []api.Volume) error {
	switch outputStyle {
	case "text":
		fmt.Println("NAME                APIVERSION  KIND    SIZE(GB)  TYPE")
		fmt.Println("----                ----------  ----    --------  ----")
		for _, v := range volumes {
			size := 0
			if v.Spec.Size != nil {
				size = *v.Spec.Size
			}
			volType := ""
			if v.Spec.Type != nil {
				volType = *v.Spec.Type
			}
			fmt.Printf("%-20s  %-10s  %-7s  %-8d  %s\n", v.Metadata.Name, v.ApiVersion, v.Kind, size, volType)
		}
		return nil

	case "json":
		data, _ := json.MarshalIndent(volumes, "", "  ")
		fmt.Println(string(data))
		return nil

	case "yaml":
		data, _ := json.Marshal(volumes)
		fmt.Println(string(data))
		return nil

	default:
		return fmt.Errorf("output style must be text/json/yaml")
	}
}

func outputNetworks(networks []api.VirtualNetwork) error {
	switch outputStyle {
	case "text":
		fmt.Println("NAME                APIVERSION  KIND")
		fmt.Println("----                ----------  ----")
		for _, n := range networks {
			fmt.Printf("%-20s  %-10s  %s\n", n.Metadata.Name, n.ApiVersion, n.Kind)
		}
		return nil

	case "json":
		data, _ := json.MarshalIndent(networks, "", "  ")
		fmt.Println(string(data))
		return nil

	case "yaml":
		data, _ := json.Marshal(networks)
		fmt.Println(string(data))
		return nil

	default:
		return fmt.Errorf("output style must be text/json/yaml")
	}
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().StringVarP(&labelSelector, "selector", "l", "", "Label selector (e.g., key=value)")
}
