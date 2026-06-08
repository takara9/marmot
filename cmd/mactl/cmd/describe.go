package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var describeCmd = &cobra.Command{
	Use:   "describe RESOURCE NAME",
	Short: "Show detailed information about a resource",
	Long:  `Describe a resource (server/srv, image/img, volume/vol, network/net, gateway/gw, vpngateway/vpngw) with NAME specified. Shows formatted text output.`,
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
		case "gateway":
			return describeGateway(objectName)
		case "vpngateway":
			return describeVpnGateway(objectName)
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

	if outputStyle == "text" {
		return describeServerText(found)
	}

	return describeResource(found)
}

func describeServerText(s *api.Server) error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}

	id := "-"
	if strings.TrimSpace(s.Metadata.Id) != "" {
		id = strings.TrimSpace(s.Metadata.Id)
	}

	statusText := "-"
	statusCode := "-"
	created := "-"
	updated := "-"
	deleting := "-"
	statusMessage := "-"
	if s.Status != nil {
		statusCode = fmt.Sprintf("%d", s.Status.StatusCode)
		if s.Status.Status != nil && strings.TrimSpace(*s.Status.Status) != "" {
			statusText = strings.TrimSpace(*s.Status.Status)
		}
		if s.Status.LastUpdateTimeStamp != nil {
			updated = s.Status.LastUpdateTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if s.Status.DeletionTimeStamp != nil {
			deleting = s.Status.DeletionTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if s.Status.Message != nil && strings.TrimSpace(*s.Status.Message) != "" {
			statusMessage = strings.TrimSpace(*s.Status.Message)
		}
	}
	ct := creationTime(s.Status)
	if !ct.IsZero() {
		created = ct.Local().Format("2006-01-02 15:04:05")
	}

	nodeName := "-"
	if s.Metadata.NodeName != nil && strings.TrimSpace(*s.Metadata.NodeName) != "" {
		nodeName = strings.TrimSpace(*s.Metadata.NodeName)
	}
	uuid := "-"
	if s.Metadata.Uuid != nil && strings.TrimSpace(*s.Metadata.Uuid) != "" {
		uuid = strings.TrimSpace(*s.Metadata.Uuid)
	}
	instanceName := "-"
	if s.Metadata.InstanceName != nil && strings.TrimSpace(*s.Metadata.InstanceName) != "" {
		instanceName = strings.TrimSpace(*s.Metadata.InstanceName)
	}
	comment := "-"
	if s.Metadata.Comment != nil && strings.TrimSpace(*s.Metadata.Comment) != "" {
		comment = strings.TrimSpace(*s.Metadata.Comment)
	}

	cpu := "-"
	if s.Spec.Cpu != nil {
		cpu = fmt.Sprintf("%d", *s.Spec.Cpu)
	}
	mem := formatMemoryGB(s.Spec.Memory)
	osVariant := "-"
	if s.Spec.OsVariant != nil && strings.TrimSpace(*s.Spec.OsVariant) != "" {
		osVariant = strings.TrimSpace(*s.Spec.OsVariant)
	}

	fmt.Println("Metadata:")
	fmt.Printf("  Name:          %s\n", s.Metadata.Name)
	fmt.Printf("  Kind:          %s\n", s.Kind)
	fmt.Printf("  ID:            %s\n", id)
	fmt.Printf("  NodeName:      %s\n", nodeName)
	fmt.Printf("  UUID:          %s\n", uuid)
	fmt.Printf("  InstanceName:  %s\n", instanceName)
	fmt.Printf("  Comment:       %s\n", comment)
	fmt.Println("  Labels:")
	if s.Metadata.Labels == nil || len(*s.Metadata.Labels) == 0 {
		fmt.Println("    -")
	} else {
		keys := make([]string, 0, len(*s.Metadata.Labels))
		for k := range *s.Metadata.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("    %s: %v\n", k, (*s.Metadata.Labels)[k])
		}
	}

	fmt.Println("\nStatus:")
	fmt.Printf("  State:         %s\n", statusText)
	fmt.Printf("  StatusCode:    %s\n", statusCode)
	fmt.Printf("  Age:           %s\n", formatServerAge(s.Status))
	fmt.Printf("  Created:       %s\n", created)
	fmt.Printf("  Updated:       %s\n", updated)
	fmt.Printf("  DeletingAt:    %s\n", deleting)
	fmt.Printf("  Message:       %s\n", statusMessage)

	fmt.Println("\nSpec:")
	fmt.Printf("  CPU:           %s\n", cpu)
	fmt.Printf("  Memory:        %s GB\n", mem)
	fmt.Printf("  OS:            %s\n", osVariant)
	fmt.Printf("  IP/CIDR:       %s\n", formatServerIPCIDR(*s))

	fmt.Println("\nNetworkInterfaces:")
	printNetworkInterfacesTable(s)

	fmt.Println("\nStorage:")
	printStorageTable(s)

	return nil
}

type storageRow struct {
	role       string
	name       string
	id         string
	vType      string
	vKind      string
	size       string
	persistent string
	path       string
}

type networkInterfaceRow struct {
	index      string
	name       string
	networkID  string
	address    string
	dhcp4      string
	dhcp6      string
	macAddress string
}

func printNetworkInterfacesTable(s *api.Server) {
	if s.Spec.NetworkInterface == nil || len(*s.Spec.NetworkInterface) == 0 {
		fmt.Println("  -")
		return
	}

	rows := make([]networkInterfaceRow, 0, len(*s.Spec.NetworkInterface))
	for i, nic := range *s.Spec.NetworkInterface {
		rows = append(rows, toNetworkInterfaceRow(i+1, nic))
	}

	fmt.Println("  NO  NAME           NETWORKID      ADDRESS/CIDR             DHCP4  DHCP6  MAC")
	fmt.Println("  --  ----           ---------      ------------             -----  -----  ---")
	for _, r := range rows {
		fmt.Printf("  %-2s  %-13s  %-13s  %-23s  %-5s  %-5s  %s\n",
			r.index,
			r.name,
			r.networkID,
			r.address,
			r.dhcp4,
			r.dhcp6,
			r.macAddress,
		)
	}
}

func toNetworkInterfaceRow(index int, nic api.NetworkInterface) networkInterfaceRow {
	row := networkInterfaceRow{
		index:      fmt.Sprintf("%d", index),
		name:       "-",
		networkID:  "-",
		address:    "-",
		dhcp4:      "-",
		dhcp6:      "-",
		macAddress: "-",
	}

	if strings.TrimSpace(nic.Networkname) != "" {
		row.name = strings.TrimSpace(nic.Networkname)
	}
	if strings.TrimSpace(nic.Networkid) != "" {
		row.networkID = strings.TrimSpace(nic.Networkid)
	}
	if nic.Address != nil && strings.TrimSpace(*nic.Address) != "" {
		addr := strings.TrimSpace(*nic.Address)
		if nic.Netmasklen != nil {
			row.address = fmt.Sprintf("%s/%d", addr, *nic.Netmasklen)
		} else if nic.Netmask != nil && strings.TrimSpace(*nic.Netmask) != "" {
			row.address = fmt.Sprintf("%s/%s", addr, strings.TrimSpace(*nic.Netmask))
		} else {
			row.address = addr
		}
	}
	if nic.Dhcp4 != nil {
		row.dhcp4 = fmt.Sprintf("%t", *nic.Dhcp4)
	}
	if nic.Dhcp6 != nil {
		row.dhcp6 = fmt.Sprintf("%t", *nic.Dhcp6)
	}
	if nic.Mac != nil && strings.TrimSpace(*nic.Mac) != "" {
		row.macAddress = strings.TrimSpace(*nic.Mac)
	}

	return row
}

func printStorageTable(s *api.Server) {
	rows := make([]storageRow, 0)

	if s.Spec.BootVolume != nil {
		rows = append(rows, toStorageRow("boot", s.Spec.BootVolume))
	}
	if s.Spec.Storage != nil {
		for i := range *s.Spec.Storage {
			rows = append(rows, toStorageRow(fmt.Sprintf("data%d", i+1), &(*s.Spec.Storage)[i]))
		}
	}

	if len(rows) == 0 {
		fmt.Println("  -")
		return
	}

	fmt.Println("  ROLE    NAME            ID      TYPE   KIND   SIZE   PERSIST  PATH")
	fmt.Println("  ----    ----            --      ----   ----   ----   -------  ----")
	for _, r := range rows {
		fmt.Printf("  %-6s  %-14s  %-6s  %-5s  %-5s  %-5s  %-7s  %s\n",
			r.role,
			r.name,
			r.id,
			r.vType,
			r.vKind,
			r.size,
			r.persistent,
			r.path,
		)
	}
}

func toStorageRow(role string, v *api.Volume) storageRow {
	r := storageRow{
		role:       role,
		name:       "-",
		id:         "-",
		vType:      "-",
		vKind:      "-",
		size:       "-",
		persistent: "-",
		path:       "-",
	}
	if v == nil {
		return r
	}

	if strings.TrimSpace(v.Metadata.Name) != "" {
		r.name = strings.TrimSpace(v.Metadata.Name)
	}
	if strings.TrimSpace(v.Metadata.Id) != "" {
		r.id = strings.TrimSpace(v.Metadata.Id)
	}
	if v.Spec.Type != nil && strings.TrimSpace(*v.Spec.Type) != "" {
		r.vType = strings.TrimSpace(*v.Spec.Type)
	}
	if v.Spec.Kind != nil && strings.TrimSpace(*v.Spec.Kind) != "" {
		r.vKind = strings.TrimSpace(*v.Spec.Kind)
	}
	if v.Spec.Size != nil {
		r.size = fmt.Sprintf("%dGB", *v.Spec.Size)
	}
	if v.Spec.Persistent != nil {
		r.persistent = fmt.Sprintf("%t", *v.Spec.Persistent)
	}
	if v.Spec.Path != nil && strings.TrimSpace(*v.Spec.Path) != "" {
		r.path = strings.TrimSpace(*v.Spec.Path)
	}

	return r
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

	if outputStyle == "text" {
		return describeImageText(found)
	}

	return describeResource(found)
}

func describeImageText(img *api.Image) error {
	if img == nil {
		return fmt.Errorf("image is nil")
	}

	id := "-"
	if strings.TrimSpace(img.Metadata.Id) != "" {
		id = strings.TrimSpace(img.Metadata.Id)
	}
	nodeName := "-"
	if img.Metadata.NodeName != nil && strings.TrimSpace(*img.Metadata.NodeName) != "" {
		nodeName = strings.TrimSpace(*img.Metadata.NodeName)
	}
	comment := "-"
	if img.Metadata.Comment != nil && strings.TrimSpace(*img.Metadata.Comment) != "" {
		comment = strings.TrimSpace(*img.Metadata.Comment)
	}

	statusText := "-"
	statusCode := "-"
	created := "-"
	updated := "-"
	deleting := "-"
	statusMessage := "-"
	if img.Status != nil {
		statusCode = fmt.Sprintf("%d", img.Status.StatusCode)
		if img.Status.Status != nil && strings.TrimSpace(*img.Status.Status) != "" {
			statusText = strings.TrimSpace(*img.Status.Status)
		}
		if img.Status.LastUpdateTimeStamp != nil {
			updated = img.Status.LastUpdateTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if img.Status.DeletionTimeStamp != nil {
			deleting = img.Status.DeletionTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if img.Status.Message != nil && strings.TrimSpace(*img.Status.Message) != "" {
			statusMessage = strings.TrimSpace(*img.Status.Message)
		}
	}
	ct := creationTime(img.Status)
	if !ct.IsZero() {
		created = ct.Local().Format("2006-01-02 15:04:05")
	}

	fmt.Println("Metadata:")
	fmt.Printf("  Name:          %s\n", img.Metadata.Name)
	fmt.Printf("  Kind:          %s\n", img.Kind)
	fmt.Printf("  ID:            %s\n", id)
	fmt.Printf("  NodeName:      %s\n", nodeName)
	fmt.Printf("  Comment:       %s\n", comment)
	fmt.Println("  Labels:")
	if img.Metadata.Labels == nil || len(*img.Metadata.Labels) == 0 {
		fmt.Println("    -")
	} else {
		keys := make([]string, 0, len(*img.Metadata.Labels))
		for k := range *img.Metadata.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("    %s: %v\n", k, (*img.Metadata.Labels)[k])
		}
	}

	fmt.Println("\nStatus:")
	fmt.Printf("  State:         %s\n", statusText)
	fmt.Printf("  StatusCode:    %s\n", statusCode)
	fmt.Printf("  Age:           %s\n", formatServerAge(img.Status))
	fmt.Printf("  Created:       %s\n", created)
	fmt.Printf("  Updated:       %s\n", updated)
	fmt.Printf("  DeletingAt:    %s\n", deleting)
	fmt.Printf("  Message:       %s\n", statusMessage)

	fmt.Println("\nSpec:")
	fmt.Printf("  Type:          %s\n", strOrDash(img.Spec.Type))
	fmt.Printf("  Kind:          %s\n", strOrDash(img.Spec.Kind))
	fmt.Printf("  Size:          %s\n", sizeOrDash(img.Spec.Size))
	fmt.Printf("  SourceUrl:     %s\n", strOrDash(img.Spec.SourceUrl))
	fmt.Printf("  Qcow2Path:     %s\n", strOrDash(img.Spec.Qcow2Path))
	fmt.Printf("  LvPath:        %s\n", strOrDash(img.Spec.LvPath))
	fmt.Printf("  VolumeGroup:   %s\n", strOrDash(img.Spec.VolumeGroup))
	fmt.Printf("  LogicalVolume: %s\n", strOrDash(img.Spec.LogicalVolume))

	return nil
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

	if outputStyle == "text" {
		return describeVolumeText(found)
	}

	return describeResource(found)
}

func describeVolumeText(v *api.Volume) error {
	if v == nil {
		return fmt.Errorf("volume is nil")
	}

	id := "-"
	if strings.TrimSpace(v.Metadata.Id) != "" {
		id = strings.TrimSpace(v.Metadata.Id)
	}
	nodeName := "-"
	if v.Metadata.NodeName != nil && strings.TrimSpace(*v.Metadata.NodeName) != "" {
		nodeName = strings.TrimSpace(*v.Metadata.NodeName)
	}
	comment := "-"
	if v.Metadata.Comment != nil && strings.TrimSpace(*v.Metadata.Comment) != "" {
		comment = strings.TrimSpace(*v.Metadata.Comment)
	}

	statusText := "-"
	statusCode := "-"
	created := "-"
	updated := "-"
	deleting := "-"
	statusMessage := "-"
	if v.Status != nil {
		statusCode = fmt.Sprintf("%d", v.Status.StatusCode)
		if v.Status.Status != nil && strings.TrimSpace(*v.Status.Status) != "" {
			statusText = strings.TrimSpace(*v.Status.Status)
		}
		if v.Status.LastUpdateTimeStamp != nil {
			updated = v.Status.LastUpdateTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if v.Status.DeletionTimeStamp != nil {
			deleting = v.Status.DeletionTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if v.Status.Message != nil && strings.TrimSpace(*v.Status.Message) != "" {
			statusMessage = strings.TrimSpace(*v.Status.Message)
		}
	}
	ct := creationTime(v.Status)
	if !ct.IsZero() {
		created = ct.Local().Format("2006-01-02 15:04:05")
	}

	fmt.Println("Metadata:")
	fmt.Printf("  Name:          %s\n", v.Metadata.Name)
	fmt.Printf("  Kind:          %s\n", v.Kind)
	fmt.Printf("  ID:            %s\n", id)
	fmt.Printf("  NodeName:      %s\n", nodeName)
	fmt.Printf("  Comment:       %s\n", comment)
	fmt.Println("  Labels:")
	if v.Metadata.Labels == nil || len(*v.Metadata.Labels) == 0 {
		fmt.Println("    -")
	} else {
		keys := make([]string, 0, len(*v.Metadata.Labels))
		for k := range *v.Metadata.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("    %s: %v\n", k, (*v.Metadata.Labels)[k])
		}
	}

	fmt.Println("\nStatus:")
	fmt.Printf("  State:         %s\n", statusText)
	fmt.Printf("  StatusCode:    %s\n", statusCode)
	fmt.Printf("  Age:           %s\n", formatServerAge(v.Status))
	fmt.Printf("  Created:       %s\n", created)
	fmt.Printf("  Updated:       %s\n", updated)
	fmt.Printf("  DeletingAt:    %s\n", deleting)
	fmt.Printf("  Message:       %s\n", statusMessage)

	fmt.Println("\nSpec:")
	fmt.Printf("  Type:          %s\n", strOrDash(v.Spec.Type))
	fmt.Printf("  Kind:          %s\n", strOrDash(v.Spec.Kind))
	fmt.Printf("  Size:          %s\n", sizeOrDash(v.Spec.Size))
	fmt.Printf("  Path:          %s\n", strOrDash(v.Spec.Path))
	fmt.Printf("  VolumeGroup:   %s\n", strOrDash(v.Spec.VolumeGroup))
	fmt.Printf("  LogicalVolume: %s\n", strOrDash(v.Spec.LogicalVolume))
	fmt.Printf("  OsVariant:     %s\n", strOrDash(v.Spec.OsVariant))
	fmt.Printf("  Persistent:    %s\n", boolOrDash(v.Spec.Persistent))
	fmt.Printf("  Iscsi:         %s\n", boolOrDash(v.Spec.Iscsi))
	fmt.Printf("  IscsiTarget:   %s\n", strOrDash(v.Spec.IscsiTargetIqn))

	return nil
}

func sizeOrDash(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%dGB", *v)
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

	if outputStyle == "text" {
		return describeNetworkText(found)
	}

	return describeResource(found)
}

func describeNetworkText(n *api.VirtualNetwork) error {
	if n == nil {
		return fmt.Errorf("network is nil")
	}

	id := "-"
	if strings.TrimSpace(n.Metadata.Id) != "" {
		id = strings.TrimSpace(n.Metadata.Id)
	}

	nodeName := "-"
	if n.Metadata.NodeName != nil && strings.TrimSpace(*n.Metadata.NodeName) != "" {
		nodeName = strings.TrimSpace(*n.Metadata.NodeName)
	}
	comment := "-"
	if n.Metadata.Comment != nil && strings.TrimSpace(*n.Metadata.Comment) != "" {
		comment = strings.TrimSpace(*n.Metadata.Comment)
	}

	statusText := "-"
	statusCode := "-"
	created := "-"
	updated := "-"
	deleting := "-"
	statusMessage := "-"
	if n.Status != nil {
		statusCode = fmt.Sprintf("%d", n.Status.StatusCode)
		if n.Status.Status != nil && strings.TrimSpace(*n.Status.Status) != "" {
			statusText = strings.TrimSpace(*n.Status.Status)
		}
		if n.Status.LastUpdateTimeStamp != nil {
			updated = n.Status.LastUpdateTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if n.Status.DeletionTimeStamp != nil {
			deleting = n.Status.DeletionTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if n.Status.Message != nil && strings.TrimSpace(*n.Status.Message) != "" {
			statusMessage = strings.TrimSpace(*n.Status.Message)
		}
	}
	ct := creationTime(n.Status)
	if !ct.IsZero() {
		created = ct.Local().Format("2006-01-02 15:04:05")
	}

	ipCidr := "-"
	if n.Spec.IpAddress != nil && strings.TrimSpace(*n.Spec.IpAddress) != "" {
		ipCidr = strings.TrimSpace(*n.Spec.IpAddress)
		if n.Spec.Netmask != nil && strings.TrimSpace(*n.Spec.Netmask) != "" {
			ipCidr = fmt.Sprintf("%s/%s", ipCidr, strings.TrimSpace(*n.Spec.Netmask))
		}
	}

	fmt.Println("Metadata:")
	fmt.Printf("  Name:          %s\n", n.Metadata.Name)
	fmt.Printf("  Kind:          %s\n", n.Kind)
	fmt.Printf("  ID:            %s\n", id)
	fmt.Printf("  NodeName:      %s\n", nodeName)
	fmt.Printf("  Comment:       %s\n", comment)
	fmt.Println("  Labels:")
	if n.Metadata.Labels == nil || len(*n.Metadata.Labels) == 0 {
		fmt.Println("    -")
	} else {
		keys := make([]string, 0, len(*n.Metadata.Labels))
		for k := range *n.Metadata.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("    %s: %v\n", k, (*n.Metadata.Labels)[k])
		}
	}

	fmt.Println("\nStatus:")
	fmt.Printf("  State:         %s\n", statusText)
	fmt.Printf("  StatusCode:    %s\n", statusCode)
	fmt.Printf("  Age:           %s\n", formatServerAge(n.Status))
	fmt.Printf("  Created:       %s\n", created)
	fmt.Printf("  Updated:       %s\n", updated)
	fmt.Printf("  DeletingAt:    %s\n", deleting)
	fmt.Printf("  Message:       %s\n", statusMessage)

	fmt.Println("\nSpec:")
	fmt.Printf("  NetworkID:     %s\n", strOrDash(n.Spec.IpNetworkId))
	fmt.Printf("  BridgeName:    %s\n", strOrDash(n.Spec.BridgeName))
	fmt.Printf("  ForwardMode:   %s\n", strOrDash(n.Spec.ForwardMode))
	fmt.Printf("  IP/CIDR:       %s\n", ipCidr)
	fmt.Printf("  Network:       %s\n", strOrDash(n.Spec.IPNetworkAddress))
	fmt.Printf("  GatewayIP:     %s\n", strOrDash(n.Spec.IpAddress))
	fmt.Printf("  DHCP:          %s\n", boolOrDash(n.Spec.Dhcp))
	fmt.Printf("  DHCP-Range:    %s - %s\n", strOrDash(n.Spec.DhcpStartAddress), strOrDash(n.Spec.DhcpEndAddress))
	fmt.Printf("  NAT:           %s\n", boolOrDash(n.Spec.Nat))
	fmt.Printf("  STP:           %s\n", boolOrDash(n.Spec.Stp))
	fmt.Printf("  OverlayMode:   %s\n", overlayWithSemantics(n.Spec.OverlayMode))
	fmt.Printf("  PeerPolicy:    %s\n", peerPolicyWithSemantics(n.Spec.OverlayMode, n.Spec.PeerPolicy))
	fmt.Printf("  UnderlayIf:    %s\n", strOrDash(n.Spec.UnderlayInterface))
	fmt.Printf("  VNI:           %s\n", vniWithSemantics(n.Spec.OverlayMode, n.Spec.Vni))

	return nil
}

func strOrDash(v *string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return "-"
	}
	return strings.TrimSpace(*v)
}

func intOrDash(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func boolOrDash(v *bool) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%t", *v)
}

func overlayOrDash(v *api.VirtualNetworkSpecOverlayMode) string {
	if v == nil {
		return "-"
	}
	return string(*v)
}

func overlayWithSemantics(v *api.VirtualNetworkSpecOverlayMode) string {
	mode := overlayOrDash(v)
	if mode == "-" {
		return mode
	}
	if strings.EqualFold(mode, string(api.Geneve)) {
		return mode + " (OVN-managed)"
	}
	if strings.EqualFold(mode, string(api.Vxlan)) {
		return mode + " (deprecated)"
	}
	return mode
}

func peerPolicyOrDash(v *api.VirtualNetworkSpecPeerPolicy) string {
	if v == nil {
		return "-"
	}
	return string(*v)
}

func peerPolicyWithSemantics(overlay *api.VirtualNetworkSpecOverlayMode, peerPolicy *api.VirtualNetworkSpecPeerPolicy) string {
	value := peerPolicyOrDash(peerPolicy)
	if overlay == nil {
		return value
	}
	if strings.EqualFold(string(*overlay), string(api.Geneve)) {
		if value == "-" {
			return "- (ignored in OVN-managed mode)"
		}
		return value + " (ignored in OVN-managed mode)"
	}
	return value
}

func vniWithSemantics(overlay *api.VirtualNetworkSpecOverlayMode, vni *int) string {
	value := intOrDash(vni)
	if overlay == nil {
		return value
	}
	if strings.EqualFold(string(*overlay), string(api.Geneve)) {
		if value == "-" {
			return "- (optional)"
		}
		return value + " (optional)"
	}
	return value
}

func describeGateway(name string) error {
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
		return fmt.Errorf("multiple gateways found with name %q; please query by id via API", name)
	}

	found := matches[0]
	if outputStyle == "text" {
		return describeGatewayText(&found)
	}

	return describeResource(found)
}

func describeGatewayText(g *api.Gateway) error {
	if g == nil {
		return fmt.Errorf("gateway is nil")
	}

	id := "-"
	if strings.TrimSpace(g.Metadata.Id) != "" {
		id = strings.TrimSpace(g.Metadata.Id)
	}

	nodeName := "-"
	if g.Metadata.NodeName != nil && strings.TrimSpace(*g.Metadata.NodeName) != "" {
		nodeName = strings.TrimSpace(*g.Metadata.NodeName)
	}

	statusText := "-"
	statusCode := "-"
	created := "-"
	updated := "-"
	deleting := "-"
	statusMessage := "-"
	if g.Status != nil {
		statusCode = fmt.Sprintf("%d", g.Status.StatusCode)
		if g.Status.Status != nil && strings.TrimSpace(*g.Status.Status) != "" {
			statusText = strings.TrimSpace(*g.Status.Status)
		}
		if g.Status.LastUpdateTimeStamp != nil {
			updated = g.Status.LastUpdateTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if g.Status.DeletionTimeStamp != nil {
			deleting = g.Status.DeletionTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if g.Status.Message != nil && strings.TrimSpace(*g.Status.Message) != "" {
			statusMessage = strings.TrimSpace(*g.Status.Message)
		}
	}
	ct := creationTime(g.Status)
	if !ct.IsZero() {
		created = ct.Local().Format("2006-01-02 15:04:05")
	}

	fmt.Println("Metadata:")
	fmt.Printf("  Name:          %s\n", g.Metadata.Name)
	fmt.Printf("  Kind:          %s\n", g.Kind)
	fmt.Printf("  ID:            %s\n", id)
	fmt.Printf("  NodeName:      %s\n", nodeName)

	fmt.Println("\nStatus:")
	fmt.Printf("  State:         %s\n", statusText)
	fmt.Printf("  StatusCode:    %s\n", statusCode)
	fmt.Printf("  Age:           %s\n", formatServerAge(g.Status))
	fmt.Printf("  Created:       %s\n", created)
	fmt.Printf("  Updated:       %s\n", updated)
	fmt.Printf("  DeletingAt:    %s\n", deleting)
	fmt.Printf("  Message:       %s\n", statusMessage)

	fmt.Println("\nSpec:")
	fmt.Printf("  BindPublicIP:  %s\n", stringOrDash(g.Spec.BindPublicIpAddress))
	fmt.Printf("  InternalServer:%s\n", stringOrDash(g.Spec.InternalServerName))
	fmt.Printf("  InternalVNet:  %s\n", stringOrDash(g.Spec.InternalVirtualNetwork))
	fmt.Printf("  RemoteCIDRs:   %s\n", gatewayRemoteCIDRsOrDefault(g.Spec.RemoteCIDRs, g.Spec.RemoteCIDR))
	fmt.Printf("  ServerPorts:   %s\n", strings.Join(orDashSlice(g.Spec.ServerPorts), ", "))

	return nil
}

func describeVpnGateway(name string) error {
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
		return fmt.Errorf("multiple vpn gateways found with name %q; please query by id via API", name)
	}

	found := matches[0]
	if outputStyle == "text" {
		return describeVpnGatewayText(&found)
	}

	return describeResource(found)
}

func describeVpnGatewayText(g *api.VpnGateway) error {
	if g == nil {
		return fmt.Errorf("vpn gateway is nil")
	}

	id := "-"
	if strings.TrimSpace(g.Metadata.Id) != "" {
		id = strings.TrimSpace(g.Metadata.Id)
	}

	nodeName := "-"
	if g.Metadata.NodeName != nil && strings.TrimSpace(*g.Metadata.NodeName) != "" {
		nodeName = strings.TrimSpace(*g.Metadata.NodeName)
	}

	statusText := "-"
	statusCode := "-"
	created := "-"
	updated := "-"
	deleting := "-"
	statusMessage := "-"
	if g.Status != nil {
		statusCode = fmt.Sprintf("%d", g.Status.StatusCode)
		if g.Status.Status != nil && strings.TrimSpace(*g.Status.Status) != "" {
			statusText = strings.TrimSpace(*g.Status.Status)
		}
		if g.Status.LastUpdateTimeStamp != nil {
			updated = g.Status.LastUpdateTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if g.Status.DeletionTimeStamp != nil {
			deleting = g.Status.DeletionTimeStamp.Local().Format("2006-01-02 15:04:05")
		}
		if g.Status.Message != nil && strings.TrimSpace(*g.Status.Message) != "" {
			statusMessage = strings.TrimSpace(*g.Status.Message)
		}
	}
	ct := creationTime(g.Status)
	if !ct.IsZero() {
		created = ct.Local().Format("2006-01-02 15:04:05")
	}

	fmt.Println("Metadata:")
	fmt.Printf("  Name:          %s\n", g.Metadata.Name)
	fmt.Printf("  Kind:          %s\n", g.Kind)
	fmt.Printf("  ID:            %s\n", id)
	fmt.Printf("  NodeName:      %s\n", nodeName)

	fmt.Println("\nStatus:")
	fmt.Printf("  State:         %s\n", statusText)
	fmt.Printf("  StatusCode:    %s\n", statusCode)
	fmt.Printf("  Age:           %s\n", formatServerAge(g.Status))
	fmt.Printf("  Created:       %s\n", created)
	fmt.Printf("  Updated:       %s\n", updated)
	fmt.Printf("  DeletingAt:    %s\n", deleting)
	fmt.Printf("  Message:       %s\n", statusMessage)

	fmt.Println("\nSpec:")
	fmt.Printf("  BindPublicIP:  %s\n", stringOrDash(g.Spec.BindPublicIpAddress))
	fmt.Printf("  InternalVNet:  %s\n", stringOrDash(g.Spec.InternalVirtualNetwork))
	fmt.Printf("  RemoteCIDRs:   %s\n", strings.Join(orDashSlicePtr(g.Spec.RemoteCIDRs), ", "))

	return nil
}

func orDashSlice(v []string) []string {
	if len(v) == 0 {
		return []string{"-"}
	}
	out := make([]string, 0, len(v))
	for _, item := range v {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{"-"}
	}
	return out
}

func orDashSlicePtr(v *[]string) []string {
	if v == nil {
		return []string{"-"}
	}
	return orDashSlice(*v)
}

func stringOrDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func gatewayRemoteCIDRsOrDefault(values *[]string, legacy *string) string {
	items := make([]string, 0)
	if values != nil {
		items = make([]string, 0, len(*values)+1)
		for _, v := range *values {
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
	}
	if len(items) == 0 && legacy != nil {
		if trimmed := strings.TrimSpace(*legacy); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	if len(items) == 0 {
		return "0.0.0.0/0"
	}
	return strings.Join(items, ", ")
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
