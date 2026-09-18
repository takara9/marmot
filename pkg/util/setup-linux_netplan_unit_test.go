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

func TestCreateNetplanInterfacesRejectsDefaultRouteViaNetworkAddress(t *testing.T) {
	mountPoint := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mountPoint, "etc", "netplan"), 0755); err != nil {
		t.Fatal(err)
	}

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

	err := CreateNetplanInterfaces(requestConfig, mountPoint)
	if err == nil {
		t.Fatal("CreateNetplanInterfaces() error = nil, want network address gateway validation error")
	}
	if !strings.Contains(err.Error(), "must not be the network address") {
		t.Fatalf("CreateNetplanInterfaces() error = %q, want network address validation", err)
	}
}

func TestCreateNetplanInterfacesAcceptsValidDefaultRoute(t *testing.T) {
	mountPoint := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mountPoint, "etc", "netplan"), 0755); err != nil {
		t.Fatal(err)
	}

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
		},
	}

	if err := CreateNetplanInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetplanInterfaces() unexpected error = %v", err)
	}
}

func TestCreateNetplanInterfacesHonorsDhcp4Only(t *testing.T) {
	mountPoint := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mountPoint, "etc", "netplan"), 0755); err != nil {
		t.Fatal(err)
	}

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Dhcp4:       BoolPtr(true),
			Dhcp6:       BoolPtr(false),
		},
	}

	if err := CreateNetplanInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetplanInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(mountPoint, "etc", "netplan", "00-nic.yaml"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "dhcp4: true") {
		t.Fatalf("netplan missing dhcp4: true, got:\n%s", got)
	}
	if !strings.Contains(got, "dhcp6: false") {
		t.Fatalf("netplan missing dhcp6: false, got:\n%s", got)
	}
	if !strings.Contains(got, "accept-ra: false") {
		t.Fatalf("netplan missing accept-ra: false when dhcp6 is disabled, got:\n%s", got)
	}
}

func TestCreateNetplanInterfacesHonorsDhcp6Only(t *testing.T) {
	mountPoint := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mountPoint, "etc", "netplan"), 0755); err != nil {
		t.Fatal(err)
	}

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
			Dhcp4:       BoolPtr(false),
			Dhcp6:       BoolPtr(true),
		},
	}

	if err := CreateNetplanInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetplanInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(mountPoint, "etc", "netplan", "00-nic.yaml"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "dhcp4: false") {
		t.Fatalf("netplan missing dhcp4: false, got:\n%s", got)
	}
	if !strings.Contains(got, "dhcp6: true") {
		t.Fatalf("netplan missing dhcp6: true, got:\n%s", got)
	}
	if strings.Contains(got, "accept-ra") {
		t.Fatalf("netplan should not set accept-ra when dhcp6 is enabled, got:\n%s", got)
	}
}

func TestCreateNetplanInterfacesDefaultsDhcpWhenFlagsOmitted(t *testing.T) {
	mountPoint := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mountPoint, "etc", "netplan"), 0755); err != nil {
		t.Fatal(err)
	}

	requestConfig := []api.NetworkInterface{
		{
			Networkname: "host-bridge",
		},
	}

	if err := CreateNetplanInterfaces(requestConfig, mountPoint); err != nil {
		t.Fatalf("CreateNetplanInterfaces() unexpected error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(mountPoint, "etc", "netplan", "00-nic.yaml"))
	if err != nil {
		t.Fatalf("ReadFile() unexpected error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "dhcp4: true") {
		t.Fatalf("netplan missing dhcp4: true, got:\n%s", got)
	}
	if !strings.Contains(got, "dhcp6: true") {
		t.Fatalf("netplan missing dhcp6: true, got:\n%s", got)
	}
}
