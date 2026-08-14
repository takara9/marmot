package controller

import (
	"errors"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
)

// DeprovisionKubernetesEngineControlPlane は、systemdユニット削除などの一部の解体ステップが
// 失敗しても、IPAM上のコントロールプレーンIPを必ず解放することを確認する。解放されないと
// CheckIPnetInUse() が恒久的にtrueを返し、専用ネットワークの削除がブロックされ続けてしまう。
func TestDeprovisionKubernetesEngineControlPlaneReleasesIPsDespitePartialFailure(t *testing.T) {
	database := newGatewayTestDatabase(t)
	ctrl := &kubernetesEngineController{db: database, node: "node-a"}

	ke := newTestKubernetesEngine(t, database, "deprov-test")
	id := api.KubernetesEngineID(ke)
	networkName := kubernetesEngineNetworkName(ke)

	ctrl.reconcileKubernetesEnginePending(ke)
	network, err := database.GetVirtualNetworkByName(networkName)
	if err != nil {
		t.Fatalf("GetVirtualNetworkByName(%q) failed: %v", networkName, err)
	}
	vnetId := api.VirtualNetworkID(network)

	// reconcileKubernetesEnginePending()はネットワークのDBエントリを作るだけで、
	// ブリッジ/IPネットワークの実プロビジョニングはネットワークコントローラーの
	// 役目のため、ここではDeprovision対象として必要な最小限を手動で補う。
	ipNetId, err := database.CreateIpNetwork(vnetId, &api.IPNetwork{AddressMaskLen: util.StringPtr("172.16.200.0/24")})
	if err != nil {
		t.Fatalf("CreateIpNetwork() failed: %v", err)
	}
	network.Spec.BridgeName = util.StringPtr("br-test-deprov")
	network.Spec.IpNetworkId = util.StringPtr(ipNetId)
	if err := database.UpdateVirtualNetworkById(vnetId, network); err != nil {
		t.Fatalf("UpdateVirtualNetworkById() failed: %v", err)
	}

	controlPlaneIP, _, err := database.AllocateIP(vnetId, ipNetId, "mke-control-plane-"+id)
	if err != nil {
		t.Fatalf("AllocateIP() for control plane failed: %v", err)
	}
	hostIP, _, err := database.AllocateIP(vnetId, ipNetId, "mke-control-plane-host-"+id)
	if err != nil {
		t.Fatalf("AllocateIP() for control plane host failed: %v", err)
	}
	if err := database.UpdateKubernetesEngineControlPlaneStatus(id, controlPlaneIP, hostIP, 26443, "1.30.0"); err != nil {
		t.Fatalf("UpdateKubernetesEngineControlPlaneStatus() failed: %v", err)
	}
	ke, err = database.GetKubernetesEngineById(id)
	if err != nil {
		t.Fatalf("GetKubernetesEngineById() failed: %v", err)
	}

	inUse, err := database.CheckIPnetInUse(vnetId, ipNetId)
	if err != nil {
		t.Fatalf("CheckIPnetInUse() failed: %v", err)
	}
	if !inUse {
		t.Fatalf("CheckIPnetInUse() = false before deprovision, want true")
	}

	// systemdユニット停止を失敗させ、コントロールプレーン解体の先頭ステップで
	// エラーが発生する状況を再現する。
	origStopUnit := systemdStopUnit
	systemdStopUnit = func(unit string) error { return errors.New("simulated systemctl stop failure") }
	t.Cleanup(func() { systemdStopUnit = origStopUnit })

	// namespace/OVSポート操作は実環境(root権限/OVSブリッジ)に依存するためフェイクに差し替える。
	origDeleteNamespace := controlPlaneDeleteNamespace
	origDeleteOVSPort := controlPlaneDeleteOVSPort
	controlPlaneDeleteNamespace = func(string) error { return nil }
	controlPlaneDeleteOVSPort = func(string, string) error { return nil }
	t.Cleanup(func() {
		controlPlaneDeleteNamespace = origDeleteNamespace
		controlPlaneDeleteOVSPort = origDeleteOVSPort
	})

	mkeConf := &marmotd.MKEConfig{}
	deprovErr := DeprovisionKubernetesEngineControlPlane(database, mkeConf, ke)
	if deprovErr == nil {
		t.Fatalf("DeprovisionKubernetesEngineControlPlane() error = nil, want error from simulated systemd failure")
	}

	inUse, err = database.CheckIPnetInUse(vnetId, ipNetId)
	if err != nil {
		t.Fatalf("CheckIPnetInUse() failed after deprovision: %v", err)
	}
	if inUse {
		t.Fatalf("CheckIPnetInUse() = true after deprovision, want false (IPs must be released despite partial failure)")
	}

	if inUseAddr, err := database.CheckIPaddrInUse(vnetId, ipNetId, controlPlaneIP); err != nil {
		t.Fatalf("CheckIPaddrInUse() for control plane IP failed: %v", err)
	} else if inUseAddr {
		t.Fatalf("control plane IP %s still marked in use", controlPlaneIP)
	}
	if inUseAddr, err := database.CheckIPaddrInUse(vnetId, ipNetId, hostIP); err != nil {
		t.Fatalf("CheckIPaddrInUse() for control plane host IP failed: %v", err)
	} else if inUseAddr {
		t.Fatalf("control plane host IP %s still marked in use", hostIP)
	}
}
