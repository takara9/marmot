package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

func TestBuildVpnGatewayServerSpecSetsDefaultPublicRoute(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &vpnController{
		db:     database,
		marmot: &marmotd.Marmot{NodeName: "hvc", Db: database},
	}

	hostBridge, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "host-bridge"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("10.10.0.0/24"),
		},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork(host-bridge) failed: %v", err)
	}
	ipNetID, err := database.CreateIpNetwork(api.VirtualNetworkID(hostBridge), &api.IPNetwork{AddressMaskLen: util.StringPtr("10.10.0.0/24")})
	if err != nil {
		t.Fatalf("CreateIpNetwork(host-bridge) failed: %v", err)
	}
	hostBridge.Spec.IpNetworkId = util.StringPtr(ipNetID)
	if err := database.UpdateVirtualNetworkById(api.VirtualNetworkID(hostBridge), hostBridge); err != nil {
		t.Fatalf("UpdateVirtualNetworkById(host-bridge) failed: %v", err)
	}

	if _, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "app-net"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("172.16.20.0/24"),
		},
	}); err != nil {
		t.Fatalf("CreateVirtualNetwork(app-net) failed: %v", err)
	}

	vpnGateway := api.VpnGateway{
		ApiVersion: "v1",
		Kind:       "VpnGateway",
		Metadata:   api.Metadata{Name: "vpn-gw"},
		Spec: api.VpnGatewaySpec{
			BindPublicIpAddress:    "10.10.0.40",
			InternalVirtualNetwork: "app-net",
		},
	}

	serverSpec, err := ctrl.buildVpnGatewayServerSpec(vpnGateway, "vgw-vpn-gw")
	if err != nil {
		t.Fatalf("buildVpnGatewayServerSpec() failed: %v", err)
	}
	if serverSpec.Spec.NetworkInterface == nil || len(*serverSpec.Spec.NetworkInterface) == 0 {
		t.Fatalf("NetworkInterface is empty")
	}

	publicNIC := (*serverSpec.Spec.NetworkInterface)[0]
	if publicNIC.Routes == nil || len(*publicNIC.Routes) == 0 {
		t.Fatalf("public NIC routes are empty")
	}
	route := (*publicNIC.Routes)[0]
	if route.To == nil || strings.TrimSpace(*route.To) != "default" {
		t.Fatalf("public NIC default route To = %v, want default", route.To)
	}
	if route.Via == nil || strings.TrimSpace(*route.Via) != "10.10.0.1" {
		t.Fatalf("public NIC default route Via = %v, want 10.10.0.1", route.Via)
	}
}

func TestBuildVpnGatewayServerSpecUsesCustomRoutes(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &vpnController{
		db:     database,
		marmot: &marmotd.Marmot{NodeName: "hvc", Db: database},
	}

	hostBridge, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "host-bridge"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("10.10.0.0/24"),
		},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork(host-bridge) failed: %v", err)
	}
	ipNetID, err := database.CreateIpNetwork(api.VirtualNetworkID(hostBridge), &api.IPNetwork{AddressMaskLen: util.StringPtr("10.10.0.0/24")})
	if err != nil {
		t.Fatalf("CreateIpNetwork(host-bridge) failed: %v", err)
	}
	hostBridge.Spec.IpNetworkId = util.StringPtr(ipNetID)
	if err := database.UpdateVirtualNetworkById(api.VirtualNetworkID(hostBridge), hostBridge); err != nil {
		t.Fatalf("UpdateVirtualNetworkById(host-bridge) failed: %v", err)
	}

	if _, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "app-net"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("172.16.20.0/24"),
		},
	}); err != nil {
		t.Fatalf("CreateVirtualNetwork(app-net) failed: %v", err)
	}

	to := "default"
	via := "10.10.0.254"
	vpnGateway := api.VpnGateway{
		ApiVersion: "v1",
		Kind:       "VpnGateway",
		Metadata:   api.Metadata{Name: "vpn-gw"},
		Spec: api.VpnGatewaySpec{
			BindPublicIpAddress:    "10.10.0.40",
			InternalVirtualNetwork: "app-net",
			Routes:                 &[]api.Route{{To: &to, Via: &via}},
		},
	}

	serverSpec, err := ctrl.buildVpnGatewayServerSpec(vpnGateway, "vgw-vpn-gw")
	if err != nil {
		t.Fatalf("buildVpnGatewayServerSpec() failed: %v", err)
	}
	publicNIC := (*serverSpec.Spec.NetworkInterface)[0]
	if publicNIC.Routes == nil || len(*publicNIC.Routes) != 1 {
		t.Fatalf("public NIC routes = %v, want 1 route", publicNIC.Routes)
	}
	if gotVia := strings.TrimSpace(util.OrDefault((*publicNIC.Routes)[0].Via, "")); gotVia != via {
		t.Fatalf("public NIC route via = %q, want %q", gotVia, via)
	}
}

// TestVpnGatewayConfiguringWaitsForSSHReadiness は、SSH未応答の間はansibleRetriesを消費せず
// CONFIGURINGで待機し、readiness timeout経過後は通常のリトライ経路へ移ることを確認する。
func TestVpnGatewayConfiguringWaitsForSSHReadiness(t *testing.T) {
	database := newGatewayTestDatabase(t)

	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "private.key")
	if err := os.WriteFile(keyPath, []byte("dummy-private-key"), 0o600); err != nil {
		t.Fatalf("WriteFile() failed for test private key: %v", err)
	}

	oldRunner := runVpnGatewayPlaybook
	oldDir := vpnGatewayPlaybookDir
	oldKey := vpnGatewayPrivateKeyPath
	oldProbe := isVpnGatewaySSHReachable
	t.Cleanup(func() {
		runVpnGatewayPlaybook = oldRunner
		vpnGatewayPlaybookDir = oldDir
		vpnGatewayPrivateKeyPath = oldKey
		isVpnGatewaySSHReachable = oldProbe
	})

	playbookCalls := 0
	runVpnGatewayPlaybook = func(playbookPath, gatewayAddress, privateKeyPath string) error {
		playbookCalls++
		return nil
	}
	vpnGatewayPlaybookDir = filepath.Join(tempDir, "playbooks")
	vpnGatewayPrivateKeyPath = keyPath

	reachable := false
	isVpnGatewaySSHReachable = func(address string) bool { return reachable }

	ctrl := &vpnController{
		db:     database,
		marmot: &marmotd.Marmot{NodeName: "hvc", Db: database},
	}

	hostBridge, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "host-bridge"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("10.10.0.0/24"),
		},
	})
	if err != nil {
		t.Fatalf("CreateVirtualNetwork(host-bridge) failed: %v", err)
	}
	ipNetID, err := database.CreateIpNetwork(api.VirtualNetworkID(hostBridge), &api.IPNetwork{AddressMaskLen: util.StringPtr("10.10.0.0/24")})
	if err != nil {
		t.Fatalf("CreateIpNetwork(host-bridge) failed: %v", err)
	}
	hostBridge.Spec.IpNetworkId = util.StringPtr(ipNetID)
	if err := database.UpdateVirtualNetworkById(api.VirtualNetworkID(hostBridge), hostBridge); err != nil {
		t.Fatalf("UpdateVirtualNetworkById(host-bridge) failed: %v", err)
	}

	if _, err := database.CreateVirtualNetwork(api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata:   api.Metadata{Name: "app-net"},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr("172.16.20.0/24"),
		},
	}); err != nil {
		t.Fatalf("CreateVirtualNetwork(app-net) failed: %v", err)
	}

	createdVpnGateway, err := database.CreateVpnGateway(api.VpnGateway{
		ApiVersion: "v1",
		Kind:       "VpnGateway",
		Metadata:   api.Metadata{Name: "vpn-gw-readiness"},
		Spec: api.VpnGatewaySpec{
			BindPublicIpAddress:    "10.10.0.50",
			InternalVirtualNetwork: "app-net",
		},
	})
	if err != nil {
		t.Fatalf("CreateVpnGateway() failed: %v", err)
	}
	vpnGatewayID := api.VpnGatewayID(createdVpnGateway)

	ctrl.reconcileVpnGatewayPending(createdVpnGateway)
	afterPending, err := database.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		t.Fatalf("GetVpnGatewayById() failed after pending reconcile: %v", err)
	}
	if afterPending.Status == nil || afterPending.Status.StatusCode != db.VPN_GATEWAY_PROVISIONING {
		t.Fatalf("vpn gateway status after pending reconcile = %v, want %d(PROVISIONING)", afterPending.Status, db.VPN_GATEWAY_PROVISIONING)
	}

	serverID := vpnGatewayManagedServerID(afterPending)
	if strings.TrimSpace(serverID) == "" {
		t.Fatalf("vpn gateway managed server id is empty")
	}
	if err := database.UpdateServerStatus(serverID, db.SERVER_RUNNING, ""); err != nil {
		t.Fatalf("UpdateServerStatus() failed for running transition: %v", err)
	}

	ctrl.reconcileVpnGatewayProvisioning(afterPending)
	afterProvisioning, err := database.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		t.Fatalf("GetVpnGatewayById() failed after provisioning reconcile: %v", err)
	}
	if afterProvisioning.Status == nil || afterProvisioning.Status.StatusCode != db.VPN_GATEWAY_CONFIGURING {
		t.Fatalf("vpn gateway status after provisioning reconcile = %v, want %d(CONFIGURING)", afterProvisioning.Status, db.VPN_GATEWAY_CONFIGURING)
	}

	// SSH not reachable yet: must stay in CONFIGURING without running ansible or consuming retries.
	ctrl.reconcileVpnGatewayConfiguring(afterProvisioning)
	afterFirstWait, err := database.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		t.Fatalf("GetVpnGatewayById() failed after first readiness wait: %v", err)
	}
	if afterFirstWait.Status == nil || afterFirstWait.Status.StatusCode != db.VPN_GATEWAY_CONFIGURING {
		t.Fatalf("vpn gateway status after first readiness wait = %v, want %d(CONFIGURING)", afterFirstWait.Status, db.VPN_GATEWAY_CONFIGURING)
	}
	if playbookCalls != 0 {
		t.Fatalf("playbookCalls = %d, want 0 while SSH is not reachable", playbookCalls)
	}
	if afterFirstWait.Metadata.Labels == nil {
		t.Fatalf("vpn gateway labels missing after first readiness wait")
	}
	if got := db.GetVpnGatewayAnsibleRetries(*afterFirstWait.Metadata.Labels); got != 0 {
		t.Fatalf("ansibleRetries = %d, want 0 while waiting for SSH readiness", got)
	}
	if _, hasSince := db.GetVpnGatewayConfiguringSince(*afterFirstWait.Metadata.Labels); !hasSince {
		t.Fatalf("configuringSince label was not recorded")
	}

	// Simulate the readiness grace period having elapsed: further waits must fall back to normal retry accounting.
	if err := ctrl.updateVpnGatewayLabels(vpnGatewayID, func(labels map[string]interface{}) {
		db.SetVpnGatewayConfiguringSince(labels, time.Now().Add(-2*vpnGatewaySSHReadinessTimeout))
	}); err != nil {
		t.Fatalf("updateVpnGatewayLabels() failed to backdate configuringSince: %v", err)
	}
	afterBackdate, err := database.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		t.Fatalf("GetVpnGatewayById() failed after backdating configuringSince: %v", err)
	}
	ctrl.reconcileVpnGatewayConfiguring(afterBackdate)
	afterTimeout, err := database.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		t.Fatalf("GetVpnGatewayById() failed after readiness timeout: %v", err)
	}
	if playbookCalls != 0 {
		t.Fatalf("playbookCalls = %d, want 0 after readiness timeout without ansible attempt", playbookCalls)
	}
	if got := db.GetVpnGatewayAnsibleRetries(*afterTimeout.Metadata.Labels); got != 1 {
		t.Fatalf("ansibleRetries = %d, want 1 after readiness timeout falls back to retry accounting", got)
	}
	if afterTimeout.Status == nil || afterTimeout.Status.StatusCode != db.VPN_GATEWAY_CONFIGURING {
		t.Fatalf("vpn gateway status after readiness timeout = %v, want %d(CONFIGURING)", afterTimeout.Status, db.VPN_GATEWAY_CONFIGURING)
	}

	// Once SSH becomes reachable, ansible must run and the gateway must become ACTIVE.
	reachable = true
	ctrl.reconcileVpnGatewayConfiguring(afterTimeout)
	afterActive, err := database.GetVpnGatewayById(vpnGatewayID)
	if err != nil {
		t.Fatalf("GetVpnGatewayById() failed after readiness success: %v", err)
	}
	if afterActive.Status == nil || afterActive.Status.StatusCode != db.VPN_GATEWAY_ACTIVE {
		t.Fatalf("vpn gateway status after readiness success = %v, want %d(ACTIVE)", afterActive.Status, db.VPN_GATEWAY_ACTIVE)
	}
	if playbookCalls != 1 {
		t.Fatalf("playbookCalls = %d, want 1 after SSH becomes reachable", playbookCalls)
	}
	if afterActive.Metadata.Labels != nil {
		if _, hasSince := db.GetVpnGatewayConfiguringSince(*afterActive.Metadata.Labels); hasSince {
			t.Fatalf("configuringSince label should be cleared once ACTIVE")
		}
	}
}
