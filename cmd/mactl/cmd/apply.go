package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var applyCmd = &cobra.Command{
	Use:   "apply [RESOURCE]",
	Short: "Create or update a resource from a file or stdin",
	Long:  `Apply a resource (server/srv, image/img, volume/vol, network/net, gateway/gw, vpngateway/vpngw, applicationloadbalancer/alb, networkloadbalancer/nlb) from a manifest file or stdin. Creates if not exists, updates if exists. If RESOURCE is omitted, it is inferred from manifest kind.`,
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// マニフェストファイルが指定されていない場合はエラー
		if manifestFile == "" {
			return fmt.Errorf("flag -f is required for apply command")
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
				if err := applyServer(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "image":
				if err := applyImage(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "volume":
				if err := applyVolume(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "network":
				if err := applyNetwork(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "gateway":
				if err := applyGateway(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "vpngateway":
				if err := applyVpnGateway(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "applicationloadbalancer":
				if err := applyLoadBalancer(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			case "networkloadbalancer":
				if err := applyNetworkLoadBalancer(manifest); err != nil {
					return fmt.Errorf("manifest %d: %w", index+1, err)
				}
			default:
				return fmt.Errorf("manifest %d: unknown resource type: %s", index+1, resourceName)
			}
		}

		return nil
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
		existingBody, _, getErr := m.GetServerById(existingId)
		if getErr != nil {
			return fmt.Errorf("failed to get existing server: %w", getErr)
		}
		var existingServer api.Server
		if unmarshalErr := json.Unmarshal(existingBody, &existingServer); unmarshalErr != nil {
			return fmt.Errorf("failed to parse existing server: %w", unmarshalErr)
		}
		if err := validateServerApplyForbiddenChanges(existingServer, *server); err != nil {
			return err
		}

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

	// まず API の結果を表示し、その後に必要なら ansible を実行する
	if err := processApplyResponse(byteBody, exists); err != nil {
		return err
	}
	if !exists {
		if err := maybeApplyServerAnsiblePlaybook(m, *server, byteBody); err != nil {
			return err
		}
	}
	return nil
}

func confirmDowntimeForServerApply(serverName string) (bool, error) {
	displayName := strings.TrimSpace(serverName)
	if displayName == "" {
		displayName = "(unknown)"
	}

	fmt.Printf("Warning: server %s will be stopped and restarted to apply changes. Continue? [y/N]: ", displayName)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	answer := strings.ToLower(strings.TrimSpace(input))
	return answer == "y" || answer == "yes", nil
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

	// spec.type/spec.kind の既定値を補完
	ApplyVolumeDefaults(volume)

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

func validateServerApplyForbiddenChanges(existing api.Server, desired api.Server) error {
	forbidden := make([]string, 0, 16)

	if desired.ApiVersion != "" && desired.ApiVersion != existing.ApiVersion {
		forbidden = append(forbidden, "apiVersion")
	}
	if desired.Kind != "" && desired.Kind != existing.Kind {
		forbidden = append(forbidden, "kind")
	}

	if desired.Metadata.Id != "" && desired.Metadata.Id != existing.Metadata.Id {
		forbidden = append(forbidden, "metadata.id")
	}
	if desired.Metadata.Name != "" && desired.Metadata.Name != existing.Metadata.Name {
		forbidden = append(forbidden, "metadata.name")
	}
	if desired.Metadata.InstanceName != nil && !reflect.DeepEqual(desired.Metadata.InstanceName, existing.Metadata.InstanceName) {
		forbidden = append(forbidden, "metadata.instanceName")
	}
	if desired.Metadata.Key != nil && !reflect.DeepEqual(desired.Metadata.Key, existing.Metadata.Key) {
		forbidden = append(forbidden, "metadata.key")
	}
	if desired.Metadata.NodeName != nil && !reflect.DeepEqual(desired.Metadata.NodeName, existing.Metadata.NodeName) {
		forbidden = append(forbidden, "metadata.nodeName")
	}
	if desired.Metadata.Uuid != nil && !reflect.DeepEqual(desired.Metadata.Uuid, existing.Metadata.Uuid) {
		forbidden = append(forbidden, "metadata.uuid")
	}

	if desired.Spec.Ansible != nil && !reflect.DeepEqual(desired.Spec.Ansible, existing.Spec.Ansible) {
		forbidden = append(forbidden, "spec.ansible")
	}
	if desired.Spec.Auth != nil && !reflect.DeepEqual(desired.Spec.Auth, existing.Spec.Auth) {
		forbidden = append(forbidden, "spec.auth")
	}
	if desired.Spec.BootVolume != nil && !reflect.DeepEqual(desired.Spec.BootVolume, existing.Spec.BootVolume) {
		forbidden = append(forbidden, "spec.bootVolume")
	}
	if desired.Spec.NetworkInterface != nil && !reflect.DeepEqual(desired.Spec.NetworkInterface, existing.Spec.NetworkInterface) {
		if !networkInterfacesMatchRequested(existing.Spec.NetworkInterface, desired.Spec.NetworkInterface) {
			forbidden = append(forbidden, "spec.networkInterface")
		}
	}
	if desired.Spec.OsLv != nil && !reflect.DeepEqual(desired.Spec.OsLv, existing.Spec.OsLv) {
		forbidden = append(forbidden, "spec.osLv")
	}
	if desired.Spec.OsVariant != nil && !reflect.DeepEqual(desired.Spec.OsVariant, existing.Spec.OsVariant) {
		forbidden = append(forbidden, "spec.osVariant")
	}
	if desired.Spec.OsVg != nil && !reflect.DeepEqual(desired.Spec.OsVg, existing.Spec.OsVg) {
		forbidden = append(forbidden, "spec.osVg")
	}
	if desired.Spec.Storage != nil && !reflect.DeepEqual(desired.Spec.Storage, existing.Spec.Storage) {
		forbidden = append(forbidden, "spec.storage")
	}

	if desired.Status != nil && !reflect.DeepEqual(desired.Status, existing.Status) {
		forbidden = append(forbidden, "status")
	}

	if len(forbidden) > 0 {
		return fmt.Errorf("%s の変更は許可されていません", strings.Join(forbidden, ", "))
	}

	return nil
}

func networkInterfacesMatchRequested(existingPtr, desiredPtr *[]api.NetworkInterface) bool {
	if desiredPtr == nil {
		return true
	}
	if existingPtr == nil {
		return false
	}

	existing := *existingPtr
	desired := *desiredPtr
	if len(existing) != len(desired) {
		return false
	}

	for i := range desired {
		if !networkInterfaceMatchesRequested(existing[i], desired[i]) {
			return false
		}
	}

	return true
}

func networkInterfaceMatchesRequested(existing, desired api.NetworkInterface) bool {
	if strings.TrimSpace(desired.Networkname) != "" && desired.Networkname != existing.Networkname {
		return false
	}
	if strings.TrimSpace(desired.Networkid) != "" && desired.Networkid != existing.Networkid {
		return false
	}

	if !stringPtrFieldMatchesRequested(existing.Address, desired.Address) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.Ethernet, desired.Ethernet) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.IpGateway, desired.IpGateway) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.IpNetworkId, desired.IpNetworkId) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.Mac, desired.Mac) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.Netmask, desired.Netmask) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.Portgroup, desired.Portgroup) {
		return false
	}
	if !stringPtrFieldMatchesRequested(existing.Uuid, desired.Uuid) {
		return false
	}

	if !boolPtrFieldMatchesRequested(existing.Dhcp4, desired.Dhcp4) {
		return false
	}
	if !boolPtrFieldMatchesRequested(existing.Dhcp6, desired.Dhcp6) {
		return false
	}
	if !intPtrFieldMatchesRequested(existing.Netmasklen, desired.Netmasklen) {
		return false
	}

	if desired.Nameservers != nil {
		if existing.Nameservers == nil || !reflect.DeepEqual(existing.Nameservers, desired.Nameservers) {
			return false
		}
	}
	if desired.Routes != nil {
		if existing.Routes == nil || !reflect.DeepEqual(existing.Routes, desired.Routes) {
			return false
		}
	}
	if desired.Vlans != nil {
		if existing.Vlans == nil || !reflect.DeepEqual(existing.Vlans, desired.Vlans) {
			return false
		}
	}

	return true
}

func stringPtrFieldMatchesRequested(existing, desired *string) bool {
	if desired == nil {
		return true
	}
	if existing == nil {
		return false
	}
	return *existing == *desired
}

func boolPtrFieldMatchesRequested(existing, desired *bool) bool {
	if desired == nil {
		return true
	}
	if existing == nil {
		return false
	}
	return *existing == *desired
}

func intPtrFieldMatchesRequested(existing, desired *int) bool {
	if desired == nil {
		return true
	}
	if existing == nil {
		return false
	}
	return *existing == *desired
}

func applyGateway(manifest map[string]interface{}) error {
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

	exists := false
	var existingID string
	list, _, err := m.GetGateways()
	if err == nil {
		var gateways []api.Gateway
		json.Unmarshal(list, &gateways)
		for _, g := range gateways {
			if g.Metadata.Name != gateway.Metadata.Name {
				continue
			}
			if strings.TrimSpace(g.Spec.InternalVirtualNetwork) == strings.TrimSpace(gateway.Spec.InternalVirtualNetwork) {
				exists = true
				existingID = api.GatewayID(g)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		api.SetGatewayID(gateway, existingID)
		byteBody, _, err = m.UpdateGatewayById(existingID, *gateway)
		if err != nil {
			return fmt.Errorf("failed to update gateway: %w", err)
		}
	} else {
		byteBody, _, err = m.CreateGateway(*gateway)
		if err != nil {
			return fmt.Errorf("failed to create gateway: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func applyVpnGateway(manifest map[string]interface{}) error {
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

	exists := false
	var existingID string
	list, _, err := m.GetVpnGateways()
	if err == nil {
		var items []api.VpnGateway
		json.Unmarshal(list, &items)
		for _, g := range items {
			if g.Metadata.Name != vpnGateway.Metadata.Name {
				continue
			}
			if strings.TrimSpace(g.Spec.InternalVirtualNetwork) == strings.TrimSpace(vpnGateway.Spec.InternalVirtualNetwork) {
				exists = true
				existingID = api.VpnGatewayID(g)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		api.SetVpnGatewayID(vpnGateway, existingID)
		byteBody, _, err = m.UpdateVpnGatewayById(existingID, *vpnGateway)
		if err != nil {
			return fmt.Errorf("failed to update vpn gateway: %w", err)
		}
	} else {
		byteBody, _, err = m.CreateVpnGateway(*vpnGateway)
		if err != nil {
			return fmt.Errorf("failed to create vpn gateway: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func applyLoadBalancer(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	lb, err := ManifestToApplicationLoadBalancer(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to application load balancer: %w", err)
	}

	if lb.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if lb.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(lb.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(lb.Spec.InternalVirtualNetwork) == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	exists := false
	var existingID string
	list, _, err := m.GetLoadBalancers()
	if err == nil {
		var items []api.ApplicationLoadBalancer
		json.Unmarshal(list, &items)
		for _, item := range items {
			if item.Metadata.Name != lb.Metadata.Name {
				continue
			}
			if strings.TrimSpace(item.Spec.InternalVirtualNetwork) == strings.TrimSpace(lb.Spec.InternalVirtualNetwork) {
				exists = true
				existingID = api.LoadBalancerID(item)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		api.SetLoadBalancerID(lb, existingID)
		byteBody, _, err = m.UpdateLoadBalancerById(existingID, *lb)
		if err != nil {
			return fmt.Errorf("failed to update application load balancer: %w", err)
		}
	} else {
		byteBody, _, err = m.CreateLoadBalancer(*lb)
		if err != nil {
			return fmt.Errorf("failed to create application load balancer: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func applyNetworkLoadBalancer(manifest map[string]interface{}) error {
	m, err := getClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get client config: %w", err)
	}

	nlb, err := ManifestToNetworkLoadBalancer(manifest)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to network load balancer: %w", err)
	}

	if nlb.ApiVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if nlb.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(nlb.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(nlb.Spec.InternalVirtualNetwork) == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}

	exists := false
	var existingID string
	list, _, err := m.GetNetworkLoadBalancers()
	if err == nil {
		var items []api.NetworkLoadBalancer
		json.Unmarshal(list, &items)
		for _, item := range items {
			if item.Metadata.Name != nlb.Metadata.Name {
				continue
			}
			if strings.TrimSpace(item.Spec.InternalVirtualNetwork) == strings.TrimSpace(nlb.Spec.InternalVirtualNetwork) {
				exists = true
				existingID = api.NetworkLoadBalancerID(item)
				break
			}
		}
	}

	var byteBody []byte
	if exists {
		api.SetNetworkLoadBalancerID(nlb, existingID)
		byteBody, _, err = m.UpdateNetworkLoadBalancerById(existingID, *nlb)
		if err != nil {
			return fmt.Errorf("failed to update network load balancer: %w", err)
		}
	} else {
		byteBody, _, err = m.CreateNetworkLoadBalancer(*nlb)
		if err != nil {
			return fmt.Errorf("failed to create network load balancer: %w", err)
		}
	}

	return processApplyResponse(byteBody, exists)
}

func processApplyResponse(byteBody []byte, updated bool) error {
	switch outputStyle {
	case "text":
		id, err := extractResponseID(byteBody)
		if err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		displayID := id
		if displayID == "" {
			displayID = "<nil>"
		}
		if updated {
			fmt.Printf("リソースが更新されました。ID: %s\n", displayID)
		} else {
			fmt.Printf("リソースが作成されました。ID: %s\n", displayID)
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
