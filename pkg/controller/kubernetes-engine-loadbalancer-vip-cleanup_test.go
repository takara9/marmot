package controller

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// Deleting: クラスタが払い出したVIP(host-bridge IPAM)と内部DNSエントリーが解放され、
// 他クラスタ・他ホストのエントリーには影響しないことを確認する。
func TestReleaseKubernetesEngineLoadBalancerVips(t *testing.T) {
	database := newGatewayTestDatabase(t)

	ke := newTestKubernetesEngine(t, database, "vip-cleanup")
	ke.Metadata.NodeName = util.StringPtr("host-a")

	hostBridge, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "host-bridge"},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork(host-bridge) failed: %v", err)
	}
	vnetID := api.VirtualNetworkID(hostBridge)
	ipNetID, err := database.CreateIpNetwork(vnetID, &api.IPNetwork{
		AddressMaskLen: util.StringPtr("192.168.50.0/24"),
	})
	if err != nil {
		t.Fatalf("CreateIpNetwork() failed: %v", err)
	}
	hostBridge.Spec.IpNetworkId = util.StringPtr(ipNetID)
	if err := database.UpdateVirtualNetworkById(vnetID, hostBridge); err != nil {
		t.Fatalf("UpdateVirtualNetworkById(host-bridge) failed: %v", err)
	}

	vip, _, err := database.AllocateIP(vnetID, ipNetID, "svcA.nsA.vip-cleanup.host-a.labo.local")
	if err != nil {
		t.Fatalf("AllocateIP() failed: %v", err)
	}
	if err := database.PutDnsEntryFQDN("svcA.nsA.vip-cleanup.host-a.labo.local", vip); err != nil {
		t.Fatalf("PutDnsEntryFQDN() failed: %v", err)
	}

	otherVip, _, err := database.AllocateIP(vnetID, ipNetID, "svcC.nsC.other-cluster.host-a.labo.local")
	if err != nil {
		t.Fatalf("AllocateIP() (other cluster) failed: %v", err)
	}
	if err := database.PutDnsEntryFQDN("svcC.nsC.other-cluster.host-a.labo.local", otherVip); err != nil {
		t.Fatalf("PutDnsEntryFQDN() (other cluster) failed: %v", err)
	}

	if err := releaseKubernetesEngineLoadBalancerVips(database, ke); err != nil {
		t.Fatalf("releaseKubernetesEngineLoadBalancerVips() failed: %v", err)
	}

	if _, err := database.GetDnsEntryFQDN("svcA.nsA.vip-cleanup.host-a.labo.local"); err == nil {
		t.Fatalf("DNS entry for this cluster still exists after release")
	}
	if _, err := database.GetDnsEntryFQDN("svcC.nsC.other-cluster.host-a.labo.local"); err != nil {
		t.Fatalf("DNS entry for other cluster was unexpectedly removed: %v", err)
	}

	// 冪等性: 対象が無くなった後に再実行してもエラーにならない。
	if err := releaseKubernetesEngineLoadBalancerVips(database, ke); err != nil {
		t.Fatalf("releaseKubernetesEngineLoadBalancerVips() 2nd call failed: %v", err)
	}
}

// NodeNameが未設定の場合は何もせず成功として扱う。
func TestReleaseKubernetesEngineLoadBalancerVipsNoNodeName(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ke := newTestKubernetesEngine(t, database, "no-node-name")

	if err := releaseKubernetesEngineLoadBalancerVips(database, ke); err != nil {
		t.Fatalf("releaseKubernetesEngineLoadBalancerVips() failed: %v", err)
	}
}
