package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
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
			   case "server", "node", "no":
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

func formatMemoryGB(memoryMB *int) string {
	if memoryMB == nil {
		return "-"
	}
	gb := float64(*memoryMB) / 1024.0
	if gb == math.Trunc(gb) {
		return fmt.Sprintf("%.0f", gb)
	}
	return fmt.Sprintf("%.1f", gb)
}

func formatServerIPCIDR(s api.Server) string {
	if s.Spec.NetworkInterface == nil {
		return "-"
	}

	entries := make([]string, 0)
	for _, nic := range *s.Spec.NetworkInterface {
		if nic.Address == nil || strings.TrimSpace(*nic.Address) == "" {
			continue
		}

		address := strings.TrimSpace(*nic.Address)
		if nic.Netmasklen != nil {
			entries = append(entries, fmt.Sprintf("%s/%d", address, *nic.Netmasklen))
			continue
		}
		if nic.Netmask != nil && strings.TrimSpace(*nic.Netmask) != "" {
			entries = append(entries, fmt.Sprintf("%s/%s", address, strings.TrimSpace(*nic.Netmask)))
			continue
		}
		entries = append(entries, address)
	}

	if len(entries) == 0 {
		return "-"
	}
	return strings.Join(entries, ",")
}

func formatServerAge(status *api.Status) string {
	ct := creationTime(status)
	if ct.IsZero() {
		return "-"
	}

	elapsed := time.Since(ct)
	if elapsed < 0 {
		return "0s"
	}

	if elapsed < time.Minute {
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	}
	if elapsed < 24*time.Hour {
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	}
	if elapsed < 30*24*time.Hour {
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	}

	return ct.Local().Format("2006-01-02")
}

func truncatePath(path string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(path) <= max {
		return path
	}
	base := filepath.Base(path)
	baseWithPrefix := ".../" + base
	if len(baseWithPrefix) <= max {
		return baseWithPrefix
	}
	if max <= 3 {
		return path[:max]
	}
	return "..." + path[len(path)-(max-3):]
}

// 出力関数
func outputServers(servers []api.Server) error {
	switch outputStyle {
	case "text":
		sort.SliceStable(servers, func(i, j int) bool {
			return creationTime(servers[i].Status).Before(creationTime(servers[j].Status))
		})
		//fmt.Println("NAME            NODE       STATUS     CPU  RAM(MB)  IP-ADDRESS       NETWORK")
		//fmt.Println("----            ----       ------     ---  -------  ----------       -------")
		fmt.Printf("%-15s  %-8s  %-12s  %-3s  %-7s  %-15s  %-15s\n",
			"NAME",
			"NODE",
			"STATUS",
			"CPU",
			"RAM(MB)",
			"IP-ADDRESS",
			"NETWORK",
		)
		fmt.Printf("%-15s  %-8s  %-12s  %-3s  %-7s  %-15s  %-15s\n",
			"----",
			"----",
			"------",
			"---",
			"-------",
			"----------",
			"-------",
		)

		for _, s := range servers {
			node := "-"
			if s.Metadata.NodeName != nil && *s.Metadata.NodeName != "" {
				node = *s.Metadata.NodeName
			}
			cpu := "-"
			if s.Spec.Cpu != nil {
				cpu = fmt.Sprintf("%d", *s.Spec.Cpu)
			}
			ram := "-"
			if s.Spec.Memory != nil {
				ram = fmt.Sprintf("%d", *s.Spec.Memory)
			}
			status := "-"
			if s.Status != nil && s.Status.Status != nil && *s.Status.Status != "" {
				status = *s.Status.Status
			}
			networkLines := serverNetworkLines(s)
			fmt.Printf("%-15s  %-8s  %-12s  %-3s  %-7s  %-15s  %-15s\n",
				s.Metadata.Name,
				node,
				status,
				cpu,
				ram,
				networkLines[0].address,
				networkLines[0].network,
			)
			for _, networkLine := range networkLines[1:] {
				fmt.Printf("%-15s  %-8s  %-12s  %-3s  %-7s  %-15s  %-15s\n",
					"",
					"",
					"",
					"",
					"",
					networkLine.address,
					networkLine.network,
				)
			}
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
		sort.SliceStable(images, func(i, j int) bool {
			return creationTime(images[i].Status).Before(creationTime(images[j].Status))
		})
		fmt.Println("NAME            NODE-NAME  STATUS     ROLE   LV   QCOW2  AGE")
		fmt.Println("----            ---------  ------     ----   --   -----  ---")
		for _, img := range images {
			nodeName := "-"
			if img.Metadata.NodeName != nil && strings.TrimSpace(*img.Metadata.NodeName) != "" {
				nodeName = strings.TrimSpace(*img.Metadata.NodeName)
			}

			status := "-"
			if img.Status != nil && img.Status.Status != nil && strings.TrimSpace(*img.Status.Status) != "" {
				status = strings.TrimSpace(*img.Status.Status)
			}

			role := "master"
			if img.Metadata.Labels != nil {
				if db.GetFollowerSyncRole(*img.Metadata.Labels) == "follower" {
					role = "replica"
				}
			}

			hasLV := "no"
			if (img.Spec.LvPath != nil && strings.TrimSpace(*img.Spec.LvPath) != "") ||
				(img.Spec.LogicalVolume != nil && strings.TrimSpace(*img.Spec.LogicalVolume) != "") {
				hasLV = "yes"
			}

			hasQcow2 := "no"
			if img.Spec.Qcow2Path != nil && strings.TrimSpace(*img.Spec.Qcow2Path) != "" {
				hasQcow2 = "yes"
			}

			fmt.Printf("%-14s  %-9s  %-9s  %-5s  %-3s  %-5s  %s\n",
				img.Metadata.Name,
				nodeName,
				status,
				role,
				hasLV,
				hasQcow2,
				formatServerAge(img.Status),
			)
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
		sort.SliceStable(volumes, func(i, j int) bool {
			return creationTime(volumes[i].Status).Before(creationTime(volumes[j].Status))
		})
		fmt.Println("NAME          NODE  KIND  TYPE   iSCSI  SIZE(GB)  STATUS     PATH                  AGE")
		fmt.Println("----          ----  ----  ----   -----  --------  ------     ----                  ---")
		for _, v := range volumes {
			size := 0
			if v.Spec.Size != nil {
				size = *v.Spec.Size
			}

			node := "-"
			if v.Metadata.NodeName != nil && strings.TrimSpace(*v.Metadata.NodeName) != "" {
				node = strings.TrimSpace(*v.Metadata.NodeName)
			}

			volKind := "-"
			if v.Spec.Kind != nil && strings.TrimSpace(*v.Spec.Kind) != "" {
				volKind = strings.TrimSpace(*v.Spec.Kind)
			}

			volType := ""
			if v.Spec.Type != nil {
				volType = *v.Spec.Type
			}

			iscsi := "-"
			if v.Spec.Iscsi != nil {
				iscsi = fmt.Sprintf("%t", *v.Spec.Iscsi)
			}

			status := "-"
			if v.Status != nil && v.Status.Status != nil && strings.TrimSpace(*v.Status.Status) != "" {
				status = strings.TrimSpace(*v.Status.Status)
			}

			path := "-"
			if v.Spec.Path != nil && strings.TrimSpace(*v.Spec.Path) != "" {
				path = truncatePath(strings.TrimSpace(*v.Spec.Path), 22)
			}

			fmt.Printf("%-12s  %-4s  %-4s  %-5s  %-5s  %-8d  %-9s  %-20s  %s\n",
				v.Metadata.Name,
				node,
				volKind,
				volType,
				iscsi,
				size,
				status,
				path,
				formatServerAge(v.Status),
			)
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
		sort.SliceStable(networks, func(i, j int) bool {
			return creationTime(networks[i].Status).Before(creationTime(networks[j].Status))
		})
		fmt.Printf("%-14s  %-9s  %-12s  %-12s  %-8s  %-14s\n",
			"NAME",
			"NODE",
			"BRIDGE",
			"STATUS",
			"AGE",
			"IP-NET",
		)
		fmt.Printf("%-14s  %-9s  %-12s  %-12s  %-8s  %-14s\n",
			"----",
			"---------",
			"-----------",
			"----------",
			"---",
			"--------------",
		)

		for _, n := range networks {
			nodeName := "-"
			if n.Metadata.NodeName != nil && strings.TrimSpace(*n.Metadata.NodeName) != "" {
				nodeName = strings.TrimSpace(*n.Metadata.NodeName)
			}

			bridgeName := "-"
			if n.Spec.BridgeName != nil && strings.TrimSpace(*n.Spec.BridgeName) != "" {
				bridgeName = strings.TrimSpace(*n.Spec.BridgeName)
			}

			ipNet := "-"
			if n.Spec.IPNetworkAddress != nil && strings.TrimSpace(*n.Spec.IPNetworkAddress) != "" {
				ipNet = strings.TrimSpace(*n.Spec.IPNetworkAddress)
			}

			status := "-"
			if n.Status != nil && n.Status.Status != nil && strings.TrimSpace(*n.Status.Status) != "" {
				status = strings.TrimSpace(*n.Status.Status)
			}

			fmt.Printf("%-14s  %-9s  %-12s  %-12s  %-8s  %-14s\n",
				n.Metadata.Name,
				nodeName,
				bridgeName,
				status,
				formatServerAge(n.Status),
				ipNet,
			)
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
