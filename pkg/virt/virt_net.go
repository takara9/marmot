package virt

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

func CreateVirtualNetworkXML(net api.VirtualNetwork) (*libvirtxml.Network, error) {
	// 入力チェック
	if net.Metadata == nil {
		return nil, fmt.Errorf("Metadata is required")
	}
	if net.Metadata.Name == nil {
		return nil, fmt.Errorf("Metadata.Name is required")
	}
	if net.Metadata.Uuid == nil {
		return nil, fmt.Errorf("Metadata.Uuid is required")
	}
	if net.Spec.BridgeName == nil {
		return nil, fmt.Errorf("BridgeName is required")
	}

	// デフォルト値の設定
	// フォワードモードのデフォルトはブリッジ
	if net.Spec.ForwardMode == nil {
		net.Spec.ForwardMode = util.StringPtr("bridge")
	}

	// NATを有効にする場合、フォワードモードのデフォルトはNAT
	if net.Spec.Nat != nil && *net.Spec.Nat == true {
		net.Spec.ForwardMode = util.StringPtr("nat")
	}

	// MACアドレスのデフォルトはランダム生成
	if net.Spec.MacAddress == nil {
		mac, err := util.GenerateRandomMAC()
		if err != nil {
			return nil, fmt.Errorf("Failed to generate random MAC address: %v", err)
		}
		net.Spec.MacAddress = util.StringPtr(mac.String())
	}

	netxml := &libvirtxml.Network{
		XMLName: xml.Name{
			Space: "",
			Local: *net.Metadata.Name,
		},
		Name: *net.Metadata.Name,
		UUID: *net.Metadata.Uuid,
		Bridge: &libvirtxml.NetworkBridge{
			Name: *net.Spec.BridgeName,
		},
	}

	if net.Spec.Nat == nil || !*net.Spec.Nat {
		// OVS ブリッジを利用するネットワークは、libvirt へ Open vSwitch virtualport を明示する。
		// これにより、先行作成済みブリッジを利用し、Linux bridge の新規作成競合を避ける。
		netxml.Forward = &libvirtxml.NetworkForward{
			Mode: "bridge",
		}
		netxml.VirtualPort = &libvirtxml.NetworkVirtualPort{
			Params: &libvirtxml.NetworkVirtualPortParams{
				OpenVSwitch: &libvirtxml.NetworkVirtualPortParamsOpenVSwitch{},
			},
		}
	}

	// forward mode='bridge' (OVS/Linux bridge) では libvirt が IP を管理しないため
	// <ip> 要素を含めると "Unsupported <ip> element" エラーになる。
	// NAT モードでのみ <ip> 要素を付与する（後続の NAT ブロックで上書きされる）。
	isNat := net.Spec.Nat != nil && *net.Spec.Nat

	if isNat && net.Spec.IpAddress != nil && net.Spec.Netmask != nil {
		netxml.IPs = []libvirtxml.NetworkIP{
			{
				Address: *net.Spec.IpAddress,
				Netmask: *net.Spec.Netmask,
			},
		}
	}

	if isNat {
		netxml.MAC = &libvirtxml.NetworkMAC{
			Address: *net.Spec.MacAddress,
		}
		netxml.Bridge.STP = "on"
		netxml.Bridge.Delay = "0"
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

	if isNat && net.Spec.Dhcp != nil && *net.Spec.Dhcp == true {
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

	//xml, err := netxml.Marshal()
	//if err != nil {
	//	return nil, err
	//}
	//fmt.Printf("Generated XML:\n%s\n", xml)

	return netxml, nil
}

func (l *LibVirtEp) ActivateVirtualNetworks(name string) error {

	return nil
}

func (l *LibVirtEp) InactivateVirtualNetworks(name string) error {

	return nil
}

func (l *LibVirtEp) GetVirtualNetworks() (*[]libvirt.Network, error) {
	slog.Debug("Libvirt GetVirtualNetworks called")

	var networks []libvirt.Network
	nameList, err := l.ListNetworks()
	if err != nil {
		return nil, err
	}

	for _, name := range nameList {
		net, err := l.Com.LookupNetworkByName(name)
		if err != nil {
			slog.Error("Error getting network by name", "err", err)
			continue
		}
		networks = append(networks, *net)
	}

	return &networks, nil
}

func (l *LibVirtEp) GetVirtualNetworkByName(name string) (*libvirt.Network, bool, error) {
	slog.Debug("Libvirt GetVirtualNetworkByName called", "name", name)

	net, err := l.Com.LookupNetworkByName(name)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			slog.Debug("Network not found", "name", name)
			return nil, false, nil
		}
		slog.Error("Error getting network by name", "err", err)
		return nil, false, err
	}

	return net, true, nil
}

func (l *LibVirtEp) DeleteVirtualNetwork(name string) error {
	net, found, err := l.GetVirtualNetworkByName(name)
	if err != nil {
		slog.Error("Error looking up network", "err", err)
		return err
	}
	if !found {
		slog.Debug("Network not found, nothing to delete", "name", name)
		return nil
	}
	if err = net.Destroy(); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not active") || strings.Contains(msg, "is not active") || strings.Contains(msg, "not running") {
			slog.Debug("Network already inactive, continue undefine", "name", name, "err", err)
		} else {
			slog.Error("Error destroying network", "err", err)
			return err
		}
	}
	if err := net.Undefine(); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not found") {
			slog.Debug("Network already undefined", "name", name)
			return nil
		}
		slog.Error("Error undefining network", "err", err)
		return err
	}

	return nil
}

// 仮想ネットワークの定義と開始
func (l *LibVirtEp) DefineAndStartVirtualNetwork(network libvirtxml.Network) error {
	slog.Debug("DefineAndStartVirtualNetwork called", "network", network.Name)

	xmlString, err := network.Marshal()
	if err != nil {
		slog.Error("Error marshaling network XML", "err", err)
		return err
	}

	fmt.Println("Generated Network XML:", string(xmlString))

	// Create Network
	net, err := l.Com.NetworkDefineXML(xmlString)
	if err != nil {
		slog.Error("Error defining network", "err", err)
		return err
	}

	// Start Network
	err = net.Create()
	if err != nil {
		slog.Error("Error starting network", "err", err)
		return err
	}

	//オートスタートを設定しないと、HVの再起動からの復帰時、停止している。
	err = net.SetAutostart(true)
	if err != nil {
		slog.Error("Error setting network autostart", "err", err)
		return err
	}
	defer net.Free()

	return nil
}

func (l *LibVirtEp) ListNetworks() ([]string, error) {
	var nameList []string

	networks, err := l.Com.ListNetworks()
	if err != nil {
		return nameList, err
	}

	for _, net := range networks {
		nameList = append(nameList, net)
	}
	return nameList, nil
}
