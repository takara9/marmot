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
