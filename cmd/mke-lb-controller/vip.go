package main

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

// ensureVipAddress は、指定したインターフェースにVIPを/32アドレスとして追加する。
// 既に設定済みの場合は何もしない(冪等)。HAProxyがVIPへbindできるようにするために必要。
func ensureVipAddress(ifaceName, vip string) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to find interface %q: %w", ifaceName, err)
	}
	addr, err := netlink.ParseAddr(vip + "/32")
	if err != nil {
		return fmt.Errorf("failed to parse VIP %q: %w", vip, err)
	}
	existing, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("failed to list addresses on %q: %w", ifaceName, err)
	}
	for _, e := range existing {
		if e.IP.Equal(addr.IP) {
			return nil
		}
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add VIP %s to %q: %w", vip, ifaceName, err)
	}
	return nil
}

// removeVipAddress は、指定したインターフェースからVIPアドレスを削除する。
// 既に存在しない場合も成功として扱う(冪等)。
func removeVipAddress(ifaceName, vip string) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to find interface %q: %w", ifaceName, err)
	}
	addr, err := netlink.ParseAddr(vip + "/32")
	if err != nil {
		return fmt.Errorf("failed to parse VIP %q: %w", vip, err)
	}
	existing, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("failed to list addresses on %q: %w", ifaceName, err)
	}
	found := false
	for _, e := range existing {
		if e.IP.Equal(addr.IP) {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	if err := netlink.AddrDel(link, addr); err != nil {
		return fmt.Errorf("failed to remove VIP %s from %q: %w", vip, ifaceName, err)
	}
	return nil
}
