package virt

import (
	"encoding/xml"
	"fmt"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

func CreateVirtualNetworkXML(net api.VirtualNetwork) error {
	// 入力チェック
	if net.Metadata == nil {
		return fmt.Errorf("Metadata is required")
	}
	if net.Metadata.Name == nil {
		return fmt.Errorf("Metadata.Name is required")
	}
	if net.Metadata.Uuid == nil {
		return fmt.Errorf("Metadata.Uuid is required")
	}
	if net.Spec.BridgeName == nil {
		return fmt.Errorf("BridgeName is required")
	}
	if net.Spec.IpAddress == nil {
		return fmt.Errorf("IpAddress is required")
	}
	if net.Spec.Netmask == nil {
		return fmt.Errorf("Netmask is required")
	}

	// デフォルト値の設定

	// フォワードモードのデフォルトはブリッジ
	if net.Spec.ForwardMode == nil {
		net.Spec.ForwardMode = util.StringPtr("bridge")
	}

	// フォワードモードがNATの場合、DHCPの開始・終了アドレスのデフォルトを設定
	//if *net.Spec.ForwardMode == "nat" {
	//	if net.Spec.DhcpStartAddress == nil {
	//		net.Spec.DhcpStartAddress = util.StringPtr("192.168.122.2")
	//	}
	//	if net.Spec.DhcpEndAddress == nil {
	//		net.Spec.DhcpEndAddress = util.StringPtr("192.168.122.254")
	//	}
	//}

	// MACアドレスのデフォルトはランダム生成
	if net.Spec.MacAddress == nil {
		mac, err := util.GenerateRandomMAC()
		if err != nil {
			return fmt.Errorf("Failed to generate random MAC address: %v", err)
		}
		net.Spec.MacAddress = util.StringPtr(mac.String())
	}

	netxml := &libvirtxml.Network{
		XMLName: xml.Name{
			Space: "",
			Local: *net.Metadata.Name,
		},
		UUID: *net.Metadata.Uuid,
		MAC: &libvirtxml.NetworkMAC{
			Address: *net.Spec.MacAddress,
		},
		IPs: []libvirtxml.NetworkIP{
			{
				Address: *net.Spec.IpAddress,
				Netmask: *net.Spec.Netmask,
			},
		},
		Bridge: &libvirtxml.NetworkBridge{
			Name:  *net.Spec.BridgeName,
			STP:   "on",
			Delay: "0",
		},
	}

	if net.Spec.Nat != nil && *net.Spec.Nat == true {
		netxml.Forward = &libvirtxml.NetworkForward{
			Mode: *net.Spec.ForwardMode,
			NAT: &libvirtxml.NetworkForwardNAT{
				Ports: []libvirtxml.NetworkForwardNATPort{
					{
						Start: 1024,
						End:   65535,
					},
				},
			},
		}
	}

	if net.Spec.Dhcp != nil && *net.Spec.Dhcp == true {
		netxml.IPs = []libvirtxml.NetworkIP{
			{
				Address: *net.Spec.IpAddress,
				Netmask: *net.Spec.Netmask,
				DHCP: &libvirtxml.NetworkDHCP{
					Ranges: []libvirtxml.NetworkDHCPRange{
						{
							Start: *net.Spec.DhcpStartAddress,
							End:   *net.Spec.DhcpEndAddress,
						},
					},
				},
			},
		}
	}

	xml, err := netxml.Marshal()
	if err != nil {
		return err
	}
	fmt.Printf("Generated XML:\n%s\n", xml)

	return nil
}

func (l *LibVirtEp) ActivateVirtualNetworks(name string) error {

	return nil
}

func (l *LibVirtEp) InactivateVirtualNetworks(name string) error {

	return nil
}

func (l *LibVirtEp) GetVirtualNetworks() (*[]libvirt.Network, error) {
	var networks []libvirt.Network

	return &networks, nil
}

func (l *LibVirtEp) GetVirtualNetworkByName(name string) (*libvirt.Network, error) {

	return nil, nil
}

func (l *LibVirtEp) DeleteVirtualNetwork(name string) error {

	return nil
}
