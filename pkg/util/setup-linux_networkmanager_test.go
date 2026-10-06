//go:build linux
// +build linux

package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func connFilePath(mountPoint, ifaceName string) string {
	return filepath.Join(mountPoint, "etc", "NetworkManager", "system-connections", ifaceName+".nmconnection")
}

func TestCreateNetworkManagerKeyfilesDefaultsToDhcpWhenNoConfig(t *testing.T) {
	mountPoint := t.TempDir()

	if err := CreateNetworkManagerKeyfiles(nil, mountPoint); err != nil {
		t.Fatalf("CreateNetworkManagerKeyfiles() unexpected error = %v", err)
	}

	data, err := os.ReadFile(connFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "method=auto") {
		t.Fatalf("nmconnection missing method=auto, got:\n%s", got)
	}
}

func TestCreateNetworkManagerKeyfilesRejectsDefaultRouteViaNetworkAddress(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Address:     StringPtr("192.168.1.210"),
			Netmasklen:  IntPtrInt(24),
			Routes: &[]api.Route{
				{
					To:  StringPtr("default"),
					Via: StringPtr("192.168.1.0"),
				},
			},
		},
	}

	err := CreateNetworkManagerKeyfiles(requestConfig, mountPoint)
	if err == nil {
		t.Fatal("CreateNetworkManagerKeyfiles() error = nil, want network address gateway validation error")
	}
	if !strings.Contains(err.Error(), "must not be the network address") {
		t.Fatalf("CreateNetworkManagerKeyfiles() error = %q, want network address validation", err)
	}
}

func TestCreateNetworkManagerKeyfilesWritesStaticAddressAndRoute(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Address:     StringPtr("192.168.1.210"),
			Netmasklen:  IntPtrInt(24),
			Routes: &[]api.Route{
				{
					To:  StringPtr("default"),
					Via: StringPtr("192.168.1.1"),
				},
			},
			Nameservers: &api.Nameservers{
				Addresses: &[]string{"192.168.1.8"},
				Search:    &[]string{"labo.local"},
			},
		},
	}

	if err := CreateNetworkManagerKeyfiles(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetworkManagerKeyfiles() unexpected error = %v", err)
	}

	path := connFilePath(mountPoint, "enp1s0")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() unexpected error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("nmconnection file permission = %v, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "method=manual") {
		t.Fatalf("nmconnection missing method=manual, got:\n%s", got)
	}
	if !strings.Contains(got, "address1=192.168.1.210/24") {
		t.Fatalf("nmconnection missing address1, got:\n%s", got)
	}
	if !strings.Contains(got, "route1=0.0.0.0/0,192.168.1.1") {
		t.Fatalf("nmconnection missing default route, got:\n%s", got)
	}
	if !strings.Contains(got, "dns=192.168.1.8;") {
		t.Fatalf("nmconnection missing dns, got:\n%s", got)
	}
	if !strings.Contains(got, "dns-search=labo.local;") {
		t.Fatalf("nmconnection missing dns-search, got:\n%s", got)
	}
	// 静的IPv4アドレスを指定した場合、IPv6は無効化される(CreateNetplanInterfacesと同様の挙動)。
	if !strings.Contains(got, "[ipv6]\nmethod=disabled") {
		t.Fatalf("nmconnection should disable ipv6 when a static ipv4 address is set, got:\n%s", got)
	}
}

func TestCreateNetworkManagerKeyfilesHonorsDhcpFlags(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Dhcp4:       BoolPtr(true),
			Dhcp6:       BoolPtr(false),
		},
	}

	if err := CreateNetworkManagerKeyfiles(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetworkManagerKeyfiles() unexpected error = %v", err)
	}

	data, err := os.ReadFile(connFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "[ipv4]\nmethod=auto") {
		t.Fatalf("nmconnection missing ipv4 method=auto, got:\n%s", got)
	}
	if !strings.Contains(got, "[ipv6]\nmethod=disabled") {
		t.Fatalf("nmconnection missing ipv6 method=disabled, got:\n%s", got)
	}
}

func TestCreateNetworkManagerKeyfilesMatchesByMacWhenAvailable(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Mac:         StringPtr("b6:f1:70:c3:c2:83"),
			Address:     StringPtr("192.168.1.176"),
			Netmasklen:  IntPtrInt(24),
		},
	}

	if err := CreateNetworkManagerKeyfiles(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetworkManagerKeyfiles() unexpected error = %v", err)
	}

	data, err := os.ReadFile(connFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	// Rocky Linuxのcloud imageはnet.ifnames=0でeth0/eth1になり、enp1s0はaltnameにしか
	// 残らないため、interface-nameでのマッチは使わずMACアドレスでマッチさせる必要がある
	// (issue #622)。
	if strings.Contains(got, "interface-name=") {
		t.Fatalf("nmconnection should not match by interface-name when MAC is known, got:\n%s", got)
	}
	if !strings.Contains(got, "[ethernet]\nmac-address=b6:f1:70:c3:c2:83") {
		t.Fatalf("nmconnection missing mac-address match, got:\n%s", got)
	}
}

func TestCreateNetworkManagerKeyfilesFallsBackToInterfaceNameWithoutMac(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Address:     StringPtr("192.168.1.176"),
			Netmasklen:  IntPtrInt(24),
		},
	}

	if err := CreateNetworkManagerKeyfiles(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetworkManagerKeyfiles() unexpected error = %v", err)
	}

	data, err := os.ReadFile(connFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "interface-name=enp1s0") {
		t.Fatalf("nmconnection should fall back to interface-name when MAC is unknown, got:\n%s", got)
	}
}
