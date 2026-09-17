package networkfabric

import (
	"fmt"

	"github.com/takara9/marmot/api"
	"github.com/vishvananda/netlink"
)

// HostPresencePortName はMarmotホスト自身がACL適用ネットワーク上で使用する
// 固定OVS internalポート名(issue #696)。
const HostPresencePortName = "mgmt-host"

// HostPresenceLogicalPortName はHostPresencePortNameに対応する固定OVN論理ポート名。
const HostPresenceLogicalPortName = "marmot-host-mgmt"

// EnsureHostPresencePort は、Marmotホスト自身がACL適用ネットワーク(vnet)上で
// 到達可能になるよう、ローカルのOVS internalポートを作成し指定IPを付与した上で、
// OVN論理ポートとして正式に束縛する(issue #696)。
// これにより、apt-cacher-ngなどホスト側で稼働するサービスへゲストVMからアクセスできる。
func (o *OVNFabric) EnsureHostPresencePort(vnet *api.VirtualNetwork, ipCIDR string) error {
	if !ovnCommandsAvailable() {
		return fmt.Errorf("ovn-nbctl is required to manage host presence port")
	}
	bridge := bridgeName(vnet)
	if bridge == "" {
		return fmt.Errorf("unable to determine bridge name")
	}

	if _, err := runOVSVSCTLCommand("--may-exist", "add-port", bridge, HostPresencePortName,
		"--", "set", "interface", HostPresencePortName, "type=internal"); err != nil {
		return fmt.Errorf("failed to ensure host presence OVS port %s on %s: %w", HostPresencePortName, bridge, err)
	}

	link, err := netlink.LinkByName(HostPresencePortName)
	if err != nil {
		return fmt.Errorf("failed to find host presence interface %s: %w", HostPresencePortName, err)
	}

	if err := ensureLinkAddress(link, ipCIDR); err != nil {
		return err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up host presence interface %s: %w", HostPresencePortName, err)
	}

	mac := link.Attrs().HardwareAddr.String()
	addr, err := netlink.ParseAddr(ipCIDR)
	if err != nil {
		return fmt.Errorf("invalid host presence ip %q: %w", ipCIDR, err)
	}

	if err := o.EnsureGuestLogicalPort(vnet, HostPresenceLogicalPortName, mac, addr.IP.String()); err != nil {
		return err
	}
	if _, err := runOVSVSCTLCommand("set", "interface", HostPresencePortName, "external_ids:iface-id="+HostPresenceLogicalPortName); err != nil {
		return fmt.Errorf("failed to set iface-id on host presence port %s: %w", HostPresencePortName, err)
	}

	return nil
}

// ensureLinkAddress はlinkに指定CIDRのIPアドレスが無ければ追加する(冪等)。
func ensureLinkAddress(link netlink.Link, ipCIDR string) error {
	desired, err := netlink.ParseAddr(ipCIDR)
	if err != nil {
		return fmt.Errorf("invalid host presence cidr %q: %w", ipCIDR, err)
	}

	existing, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("failed to list addresses on %s: %w", link.Attrs().Name, err)
	}
	for _, addr := range existing {
		if addr.IPNet.String() == desired.IPNet.String() {
			return nil
		}
	}

	if err := netlink.AddrAdd(link, desired); err != nil {
		return fmt.Errorf("failed to add address %s on %s: %w", ipCIDR, link.Attrs().Name, err)
	}
	return nil
}
