package marmotd

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
)

func TestIsImportExcludedNetworkName(t *testing.T) {
	cases := map[string]bool{
		"mgmt":        true,
		" mgmt ":      true,
		"host-bridge": false,
		"default":     false,
		"server-net":  false,
		"":            false,
	}
	for name, want := range cases {
		if got := isImportExcludedNetworkName(name); got != want {
			t.Errorf("isImportExcludedNetworkName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDeployVirtualNetworkRejectsManagementNetworkWithoutCIDR(t *testing.T) {
	// DB/libvirt に触れる前に拒否されるため、ゼロ値の Marmot で呼び出せる。
	m := &Marmot{}
	vnet := api.VirtualNetwork{Metadata: api.Metadata{Name: ManagementNetworkName}}

	err := m.DeployVirtualNetwork(vnet)
	if err == nil {
		t.Fatal("DeployVirtualNetwork() error = nil, want rejection for mgmt without ipNetworkAddress")
	}
	if !strings.Contains(err.Error(), ManagementNetworkCIDR) {
		t.Fatalf("error = %q, want it to mention %s", err.Error(), ManagementNetworkCIDR)
	}
}
