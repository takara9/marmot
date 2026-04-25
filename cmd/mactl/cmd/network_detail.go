package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var networkDetailCmd = &cobra.Command{
	Use:   "detail [network id]",
	Short: "show network details",
	Args:  cobra.MinimumNArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
		for _, networkId := range args {
			byteBody, _, err := m.GetVirtualNetworkById(networkId)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "GetVirtualNetworkById", "Id", networkId, "err", err)
				continue
			}

			var data api.VirtualNetwork
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", "Id", networkId, "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				ipNetworks, err := getRelatedIPNetworks(m, networkId)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "GetIpNetworksByVirtualNetworkId", "Id", networkId, "err", err)
				}
				printNetworkDetails(data, ipNetworks)
				fmt.Println()
				continue

			case "json":
				fmt.Println(string(byteBody))
				continue

			case "yaml":
				yamlBytes, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", networkId, "err", err)
					continue

				}
				fmt.Println(string(yamlBytes))
				continue

			default:
				fmt.Fprintln(cmd.ErrOrStderr(), "output style must set text/json/yaml")
				continue

			}
		}
		return nil
	},
}

func init() {
	networkCmd.AddCommand(networkDetailCmd)
}

func getRelatedIPNetworks(m *client.MarmotEndpoint, networkId string) ([]api.IPNetwork, error) {
	byteBody, _, err := m.GetIpNetworksByVirtualNetworkId(networkId)
	if err != nil {
		return nil, err
	}
	if string(byteBody) == "null\n" || string(byteBody) == "null" {
		return nil, nil
	}
	var ipNetworks []api.IPNetwork
	if err := json.Unmarshal(byteBody, &ipNetworks); err != nil {
		return nil, err
	}
	return ipNetworks, nil
}

func printNetworkDetails(network api.VirtualNetwork, ipNetworks []api.IPNetwork) {
	fmt.Println("Network Details")
	fmt.Printf("  Id:              %s\n", displayID(network.Id))
	fmt.Printf("  UUID:            %s\n", metadataString(network.Metadata, func(m *api.Metadata) *string { return m.Uuid }))
	fmt.Printf("  Name:            %s\n", metadataString(network.Metadata, func(m *api.Metadata) *string { return m.Name }))
	fmt.Printf("  Status:          %s\n", formatNetworkStatus(network.Status))
	fmt.Printf("  Created At:      %s\n", statusTime(network.Status, func(s *api.Status) *time.Time { return s.CreationTimeStamp }))
	fmt.Printf("  Last Updated:    %s\n", statusTime(network.Status, func(s *api.Status) *time.Time { return s.LastUpdateTimeStamp }))
	fmt.Printf("  Message:         %s\n", statusString(network.Status, func(s *api.Status) *string { return s.Message }))
	fmt.Println()

	fmt.Println("Connectivity")
	fmt.Printf("  Bridge Name:     %s\n", specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.BridgeName }))
	fmt.Printf("  Forward Mode:    %s\n", specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.ForwardMode }))
	fmt.Printf("  IP Address:      %s\n", specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.IpAddress }))
	fmt.Printf("  Netmask:         %s\n", specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.Netmask }))
	fmt.Printf("  MAC Address:     %s\n", specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.MacAddress }))
	fmt.Printf("  DHCP:            %s\n", specBool(network.Spec, func(s *api.VirtualNetworkSpec) *bool { return s.Dhcp }))
	fmt.Printf("  DHCP Range:      %s\n", formatRange(
		specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.DhcpStartAddress }),
		specString(network.Spec, func(s *api.VirtualNetworkSpec) *string { return s.DhcpEndAddress }),
	))
	fmt.Printf("  NAT:             %s\n", specBool(network.Spec, func(s *api.VirtualNetworkSpec) *bool { return s.Nat }))
	fmt.Printf("  STP:             %s\n", specBool(network.Spec, func(s *api.VirtualNetworkSpec) *bool { return s.Stp }))
	fmt.Println()

	fmt.Println("Related IP Networks")
	printRelatedIPNetworks(ipNetworks)
	fmt.Println()

	fmt.Println("Internal DNS")
	printNetworkDNS(network)
}

func printRelatedIPNetworks(ipNetworks []api.IPNetwork) {
	if len(ipNetworks) == 0 {
		fmt.Println("  None")
		return
	}
	for index, ipNetwork := range ipNetworks {
		fmt.Printf("  [%d]\n", index+1)
		fmt.Printf("    Id:            %s\n", displayID(ipNetwork.Id))
		fmt.Printf("    Address/Mask:  %s\n", stringOrNA(ipNetwork.AddressMaskLen))
		fmt.Printf("    Network:       %s\n", stringOrNA(ipNetwork.NetworkAddress))
		fmt.Printf("    Gateway:       %s\n", stringOrNA(ipNetwork.Gateway))
		fmt.Printf("    Netmask:       %s\n", formatIPNetmask(ipNetwork.Netmask, ipNetwork.Netmasklen))
		fmt.Printf("    Range:         %s\n", formatRange(stringOrNA(ipNetwork.StartAddress), stringOrNA(ipNetwork.EndAddress)))
	}
}

func printNetworkDNS(network api.VirtualNetwork) {
	name := metadataString(network.Metadata, func(m *api.Metadata) *string { return m.Name })
	if name == "N/A" {
		fmt.Println("  DNS Name:        N/A")
		fmt.Println("  Note:            Network name is not set, so DNS subdomain cannot be determined.")
		return
	}
	fmt.Printf("  DNS Name:        *.%s\n", name)
	fmt.Printf("  Note:            Server names on this network are registered as <hostname>.%s\n", name)
	fmt.Printf("  Subdomain:       %s\n", name)
}

func displayID(id string) string {
	if id == "" {
		return "N/A"
	}
	return id
}

func metadataString(metadata *api.Metadata, selector func(*api.Metadata) *string) string {
	if metadata == nil {
		return "N/A"
	}
	return stringOrNA(selector(metadata))
}

func specString(spec *api.VirtualNetworkSpec, selector func(*api.VirtualNetworkSpec) *string) string {
	if spec == nil {
		return "N/A"
	}
	return stringOrNA(selector(spec))
}

func statusString(status *api.Status, selector func(*api.Status) *string) string {
	if status == nil {
		return "N/A"
	}
	return stringOrNA(selector(status))
}

func specBool(spec *api.VirtualNetworkSpec, selector func(*api.VirtualNetworkSpec) *bool) string {
	if spec == nil {
		return "N/A"
	}
	value := selector(spec)
	if value == nil {
		return "N/A"
	}
	if *value {
		return "yes"
	}
	return "no"
}

func statusTime(status *api.Status, selector func(*api.Status) *time.Time) string {
	if status == nil {
		return "N/A"
	}
	value := selector(status)
	if value == nil {
		return "N/A"
	}
	return value.Local().Format(time.RFC3339)
}

func formatNetworkStatus(status *api.Status) string {
	if status == nil {
		return "N/A"
	}
	statusText := db.NetworkStatus[status.StatusCode]
	if statusText == "" && status.Status != nil {
		statusText = *status.Status
	}
	if statusText == "" {
		statusText = fmt.Sprintf("UNKNOWN(%d)", status.StatusCode)
	}
	return fmt.Sprintf("%s (%d)", statusText, status.StatusCode)
}

func stringOrNA(value *string) string {
	if value == nil || *value == "" {
		return "N/A"
	}
	return *value
}

func formatIPNetmask(netmask *string, netmasklen *int) string {
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

func formatRange(start string, end string) string {
	if start == "N/A" && end == "N/A" {
		return "N/A"
	}
	return fmt.Sprintf("%s - %s", start, end)
}
