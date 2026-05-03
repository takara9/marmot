package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var serverDetailCmd = &cobra.Command{
	Use:   "detail [server-id]",
	Short: "Show server details",
	Args:  cobra.ExactArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
		serverId := args[0]
		byteBody, _, err := m.GetServerById(serverId)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		var data interface{}
		var server api.Server
		switch outputStyle {
		case "text":
			if err := json.Unmarshal(byteBody, &server); err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Unmarshal:", err)
				return err
			}
			// 別のメソッドに切り出して、サーバーの詳細を表示する
			printServerDetails(server)
			return nil
		case "json":
			err := json.Unmarshal(byteBody, &data)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Unmarshal:", err)
				return nil
			}
			byteJson, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Marshal:", err)
				return nil
			}
			fmt.Println(string(byteJson))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Unmarshal:", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Marshal:", err)
				return err
			}
			fmt.Println(string(yamlBytes))
			return nil

		default:
			fmt.Println("output style must set text/json/yaml")
			return fmt.Errorf("output style must set text/json/yaml")
		}

	},
}

func init() {
	serverCmd.AddCommand(serverDetailCmd)
}

// サーバーの詳細をテキスト形式で表示する関数
func printServerDetails(server api.Server) {
	fmt.Println("Server Details")
	fmt.Printf("  Id:            %s\n", formatID(server.Id))
	fmt.Printf("  UUID:          %s\n", stringValue(server.Metadata, func(m *api.Metadata) *string { return m.Uuid }))
	fmt.Printf("  Name:          %s\n", stringValue(server.Metadata, func(m *api.Metadata) *string { return m.Name }))
	fmt.Printf("  Instance Name: %s\n", stringValue(server.Metadata, func(m *api.Metadata) *string { return m.InstanceName }))
	fmt.Printf("  Status:        %s\n", formatServerStatus(server.Status))
	fmt.Printf("  Message:       %s\n", stringValue(server.Status, func(s *api.Status) *string { return s.Message }))
	fmt.Println()

	fmt.Println("Lifecycle")
	fmt.Printf("  Created At:    %s\n", timeValue(server.Status, func(s *api.Status) *time.Time { return s.CreationTimeStamp }))
	fmt.Printf("  Last Updated:  %s\n", timeValue(server.Status, func(s *api.Status) *time.Time { return s.LastUpdateTimeStamp }))
	fmt.Printf("  Uptime:        %s\n", uptimeValue(server.Status))
	fmt.Println()

	fmt.Println("Resources")
	fmt.Printf("  OS Variant:    %s\n", stringValue(server.Spec, func(s *api.ServerSpec) *string { return s.OsVariant }))
	fmt.Printf("  OS Level:      %s\n", stringValue(server.Spec, func(s *api.ServerSpec) *string { return s.OsLv }))
	fmt.Printf("  CPU:           %s\n", intValue(server.Spec, func(s *api.ServerSpec) *int { return s.Cpu }, " vCPU"))
	fmt.Printf("  Memory:        %s\n", intValue(server.Spec, func(s *api.ServerSpec) *int { return s.Memory }, " MB"))
	fmt.Println()

	fmt.Println("Boot Volume")
	printVolumeSummary(server.Spec, func(s *api.ServerSpec) *api.Volume { return s.BootVolume })
	fmt.Println()

	fmt.Println("Attached Volumes")
	printVolumeList(server.Spec, func(s *api.ServerSpec) *[]api.Volume { return s.Storage })
	fmt.Println()

	fmt.Println("Network Interfaces")
	printNetworkInterfaces(server.Spec, func(s *api.ServerSpec) *[]api.NetworkInterface { return s.NetworkInterface })
}

func formatID(id string) string {
	if id == "" {
		return "N/A"
	}
	return id
}

func stringValue[T any](obj *T, selector func(*T) *string) string {
	if obj == nil {
		return "N/A"
	}
	value := selector(obj)
	if value == nil || *value == "" {
		return "N/A"
	}
	return *value
}

func intValue[T any](obj *T, selector func(*T) *int, suffix string) string {
	if obj == nil {
		return "N/A"
	}
	value := selector(obj)
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%d%s", *value, suffix)
}

func boolValue(value *bool) string {
	if value == nil {
		return "N/A"
	}
	if *value {
		return "yes"
	}
	return "no"
}

func timeValue[T any](obj *T, selector func(*T) *time.Time) string {
	if obj == nil {
		return "N/A"
	}
	value := selector(obj)
	if value == nil {
		return "N/A"
	}
	return value.Local().Format(time.RFC3339)
}

func uptimeValue(status *api.Status) string {
	if status == nil {
		return "N/A"
	}
	startedAt := status.CreationTimeStamp
	if startedAt == nil {
		startedAt = status.LastUpdateTimeStamp
	}
	if startedAt == nil || time.Since(*startedAt) < 0 {
		return "N/A"
	}
	return time.Since(*startedAt).Round(time.Second).String()
}

func formatServerStatus(status *api.Status) string {
	if status == nil {
		return "N/A"
	}
	statusText := db.ServerStatus[status.StatusCode]
	if statusText == "" && status.Status != nil {
		statusText = *status.Status
	}
	if statusText == "" {
		statusText = fmt.Sprintf("UNKNOWN(%d)", status.StatusCode)
	}
	return fmt.Sprintf("%s (%d)", statusText, status.StatusCode)
}

func printVolumeSummary(spec *api.ServerSpec, selector func(*api.ServerSpec) *api.Volume) {
	if spec == nil {
		fmt.Println("  N/A")
		return
	}
	volume := selector(spec)
	if volume == nil {
		fmt.Println("  N/A")
		return
	}
	printVolumeDetails("  ", volume)
}

func printVolumeList(spec *api.ServerSpec, selector func(*api.ServerSpec) *[]api.Volume) {
	if spec == nil {
		fmt.Println("  N/A")
		return
	}
	volumes := selector(spec)
	if volumes == nil || len(*volumes) == 0 {
		fmt.Println("  None")
		return
	}
	for index := range *volumes {
		fmt.Printf("  [%d]\n", index+1)
		printVolumeDetails("    ", &(*volumes)[index])
	}
}

func printVolumeDetails(indent string, volume *api.Volume) {
	if volume == nil {
		fmt.Printf("%sN/A\n", indent)
		return
	}
	fmt.Printf("%sId:          %s\n", indent, formatID(volume.Id))
	fmt.Printf("%sName:        %s\n", indent, stringValue(volume.Metadata, func(m *api.Metadata) *string { return m.Name }))
	fmt.Printf("%sPath:        %s\n", indent, stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.Path }))
	fmt.Printf("%sType:        %s\n", indent, stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.Type }))
	fmt.Printf("%sVolumeGroup: %s\n", indent, stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.VolumeGroup }))
	fmt.Printf("%sSize:        %s\n", indent, intValue(volume.Spec, func(s *api.VolSpec) *int { return s.Size }, " GB"))
	if volume.Spec != nil && volume.Spec.Persistent != nil {
		fmt.Printf("%sPersistent:  %s\n", indent, boolValue(volume.Spec.Persistent))
	} else {
		fmt.Printf("%sPersistent:  N/A\n", indent)
	}
}

func printNetworkInterfaces(spec *api.ServerSpec, selector func(*api.ServerSpec) *[]api.NetworkInterface) {
	if spec == nil {
		fmt.Println("  N/A")
		return
	}
	interfaces := selector(spec)
	if interfaces == nil || len(*interfaces) == 0 {
		fmt.Println("  None")
		return
	}
	for index := range *interfaces {
		nic := (*interfaces)[index]
		fmt.Printf("  [%d] %s\n", index+1, networkLabel(nic))
		fmt.Printf("      Address:     %s\n", stringPtr(nic.Address))
		fmt.Printf("      Netmask:     %s\n", formatNetmask(nic.Netmask, nic.Netmasklen))
		fmt.Printf("      Gateway:     %s\n", stringPtr(nic.IpGateway))
		fmt.Printf("      MAC:         %s\n", stringPtr(nic.Mac))
		fmt.Printf("      Ethernet:    %s\n", stringPtr(nic.Ethernet))
		fmt.Printf("      DHCPv4:      %s\n", boolValue(nic.Dhcp4))
		fmt.Printf("      DHCPv6:      %s\n", boolValue(nic.Dhcp6))
		fmt.Printf("      Port Group:  %s\n", stringPtr(nic.Portgroup))
		fmt.Printf("      VLANs:       %s\n", formatVlans(nic.Vlans))
		fmt.Printf("      Nameservers: %s\n", formatNameservers(nic.Nameservers))
	}
}

func networkLabel(nic api.NetworkInterface) string {
	parts := make([]string, 0, 2)
	if nic.Networkname != "" {
		parts = append(parts, nic.Networkname)
	}
	if nic.Networkid != "" {
		parts = append(parts, fmt.Sprintf("id=%s", nic.Networkid))
	}
	if len(parts) == 0 {
		return "N/A"
	}
	return strings.Join(parts, " ")
}

func stringPtr(value *string) string {
	if value == nil || *value == "" {
		return "N/A"
	}
	return *value
}

func formatNetmask(netmask *string, netmasklen *int) string {
	if netmask != nil && *netmask != "" && netmasklen != nil {
		return fmt.Sprintf("%s /%d", *netmask, *netmasklen)
	}
	if netmask != nil && *netmask != "" {
		return *netmask
	}
	if netmasklen != nil {
		return fmt.Sprintf("/%d", *netmasklen)
	}
	return "N/A"
}

func formatVlans(vlans *[]uint) string {
	if vlans == nil || len(*vlans) == 0 {
		return "N/A"
	}
	items := make([]string, 0, len(*vlans))
	for _, vlan := range *vlans {
		items = append(items, fmt.Sprintf("%d", vlan))
	}
	return strings.Join(items, ", ")
}

func formatNameservers(nameservers *api.Nameservers) string {
	if nameservers == nil || nameservers.Addresses == nil || len(*nameservers.Addresses) == 0 {
		return "N/A"
	}
	return strings.Join(*nameservers.Addresses, ", ")
}
