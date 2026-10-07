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

// ifupdown の source-directory はファイル名にドットを含むものを無視するため、
// 拡張子を付けない(issue #622)。
func ifupdownFilePath(mountPoint, ifaceName string) string {
	return filepath.Join(mountPoint, "etc", "network", "interfaces.d", ifaceName)
}

func TestCreateIfupdownInterfacesDefaultsToDhcpWhenNoConfig(t *testing.T) {
	mountPoint := t.TempDir()

	if err := CreateIfupdownInterfaces(nil, mountPoint); err != nil {
		t.Fatalf("CreateIfupdownInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(ifupdownFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "auto enp1s0") {
		t.Fatalf("ifupdown config missing auto enp1s0, got:\n%s", got)
	}
	if !strings.Contains(got, "iface enp1s0 inet dhcp") {
		t.Fatalf("ifupdown config missing inet dhcp, got:\n%s", got)
	}
	if !strings.Contains(got, "iface enp1s0 inet6 dhcp") {
		t.Fatalf("ifupdown config missing inet6 dhcp, got:\n%s", got)
	}
}

func TestCreateIfupdownInterfacesRejectsDefaultRouteViaNetworkAddress(t *testing.T) {
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

	err := CreateIfupdownInterfaces(requestConfig, mountPoint)
	if err == nil {
		t.Fatal("CreateIfupdownInterfaces() error = nil, want network address gateway validation error")
	}
	if !strings.Contains(err.Error(), "must not be the network address") {
		t.Fatalf("CreateIfupdownInterfaces() error = %q, want network address validation", err)
	}
}

func TestCreateIfupdownInterfacesWritesStaticAddressAndRoute(t *testing.T) {
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
				{
					To:  StringPtr("10.0.0.0/8"),
					Via: StringPtr("192.168.1.5"),
				},
			},
			Nameservers: &api.Nameservers{
				Addresses: &[]string{"192.168.1.8"},
				Search:    &[]string{"labo.local"},
			},
		},
	}

	if err := CreateIfupdownInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateIfupdownInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(ifupdownFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "iface enp1s0 inet static") {
		t.Fatalf("ifupdown config missing inet static, got:\n%s", got)
	}
	if !strings.Contains(got, "address 192.168.1.210/24") {
		t.Fatalf("ifupdown config missing address, got:\n%s", got)
	}
	if !strings.Contains(got, "gateway 192.168.1.1") {
		t.Fatalf("ifupdown config missing default gateway, got:\n%s", got)
	}
	if !strings.Contains(got, "up ip route add 10.0.0.0/8 via 192.168.1.5 dev enp1s0") {
		t.Fatalf("ifupdown config missing additional route, got:\n%s", got)
	}
	if !strings.Contains(got, "dns-nameservers 192.168.1.8") {
		t.Fatalf("ifupdown config missing dns-nameservers, got:\n%s", got)
	}
	if !strings.Contains(got, "dns-search labo.local") {
		t.Fatalf("ifupdown config missing dns-search, got:\n%s", got)
	}
}

func TestCreateIfupdownInterfacesWritesIPv6StaticAddress(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Address:     StringPtr("fd00::10"),
			Netmasklen:  IntPtrInt(64),
		},
	}

	if err := CreateIfupdownInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateIfupdownInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(ifupdownFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "iface enp1s0 inet6 static") {
		t.Fatalf("ifupdown config missing inet6 static, got:\n%s", got)
	}
	if !strings.Contains(got, "address fd00::10/64") {
		t.Fatalf("ifupdown config missing IPv6 address, got:\n%s", got)
	}
}

func TestCreateIfupdownInterfacesHonorsDhcpFlags(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Dhcp4:       BoolPtr(true),
			Dhcp6:       BoolPtr(false),
		},
	}

	if err := CreateIfupdownInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateIfupdownInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(ifupdownFilePath(mountPoint, "enp1s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "iface enp1s0 inet dhcp") {
		t.Fatalf("ifupdown config missing inet dhcp, got:\n%s", got)
	}
	if strings.Contains(got, "inet6 dhcp") {
		t.Fatalf("ifupdown config should not enable dhcp6, got:\n%s", got)
	}
}

func TestCreateIfupdownInterfacesMultipleInterfaces(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{Networkname: "mgmt", Dhcp4: BoolPtr(true), Dhcp6: BoolPtr(false)},
		{Networkname: "host-bridge", Address: StringPtr("192.168.1.176"), Netmasklen: IntPtrInt(24)},
	}

	if err := CreateIfupdownInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateIfupdownInterfaces() unexpected error = %v", err)
	}

	if _, err := os.Stat(ifupdownFilePath(mountPoint, "enp1s0")); err != nil {
		t.Fatalf("expected enp1s0 config file, stat err = %v", err)
	}
	data, err := os.ReadFile(ifupdownFilePath(mountPoint, "enp2s0"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	if !strings.Contains(string(data), "address 192.168.1.176/24") {
		t.Fatalf("ifupdown config for second interface missing address, got:\n%s", string(data))
	}
}

// ifupdown の source-directory は、ファイル名が英数字・アンダースコア・ハイフンのみで
// 構成されるものに限り読み込み、ドットを含むファイル名(例: "enp1s0.cfg")は無視して
// 設定を適用しない。この回帰を防ぐため、生成されるファイル名にドットが含まれないことを
// 明示的に検証する(issue #622)。
func TestCreateIfupdownInterfacesFileNamesContainNoDot(t *testing.T) {
	mountPoint := t.TempDir()

	requestConfig := []api.NetworkInterface{
		{Networkname: "mgmt", Dhcp4: BoolPtr(true), Dhcp6: BoolPtr(false)},
		{Networkname: "host-bridge", Address: StringPtr("192.168.1.176"), Netmasklen: IntPtrInt(24)},
	}

	if err := CreateIfupdownInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateIfupdownInterfaces() unexpected error = %v", err)
	}

	interfacesDir := filepath.Join(mountPoint, "etc", "network", "interfaces.d")
	entries, err := os.ReadDir(interfacesDir)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one generated interfaces.d file")
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".") {
			t.Fatalf("generated interfaces.d file name %q must not contain a dot, otherwise ifupdown's source-directory silently ignores it", entry.Name())
		}
	}
}

