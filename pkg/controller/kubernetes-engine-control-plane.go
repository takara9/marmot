package controller

import (
	"fmt"
	"net"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
)

const (
	DefaultKubernetesAPIServerPortRangeStart = 26443
	DefaultKubernetesAPIServerPortRangeEnd   = 27443
	DefaultKubernetesServiceClusterCIDR      = "10.96.0.0/12"
)

func AllocateKubernetesEngineAPIServerPort(engines []api.KubernetesEngine, rangeStart, rangeEnd int) (int, error) {
	reserved := make(map[int]bool)
	for _, engine := range engines {
		if engine.Status != nil && engine.Status.ApiServerPort != nil {
			reserved[*engine.Status.ApiServerPort] = true
		}
	}
	for port := rangeStart; port < rangeEnd; port++ {
		if reserved[port] || !kubernetesEnginePortProbe(port) {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free Kubernetes API server port available in range [%d, %d)", rangeStart, rangeEnd)
}

func ProvisionKubernetesEngineControlPlane(database *db.Database, mkeConf *marmotd.MKEConfig, ownEtcdURL string, ke api.KubernetesEngine) error {
	clusterName, err := validateKubernetesEngineEtcdClusterName(ke.Metadata.Name)
	if err != nil {
		return err
	}
	hostBindAddress := strings.TrimSpace(mkeConf.ControlPlaneBindAddress)
	if hostBindAddress == "" {
		return fmt.Errorf("control plane host access requires control_plane_bind_address to be configured in %s", marmotd.DefaultMKEConfigPath)
	}
	if net.ParseIP(hostBindAddress) == nil {
		return fmt.Errorf("control_plane_bind_address %q is not a valid IP address", hostBindAddress)
	}
	network, err := database.GetVirtualNetworkByName(kubernetesEngineNetworkName(ke))
	if err != nil {
		return fmt.Errorf("failed to get KubernetesEngine network: %w", err)
	}
	if !isKubernetesEngineOwnedNetwork(network, api.KubernetesEngineID(ke)) {
		return fmt.Errorf("network %s is not owned by KubernetesEngine %s", network.Metadata.Name, api.KubernetesEngineID(ke))
	}
	if network.Spec.BridgeName == nil || strings.TrimSpace(*network.Spec.BridgeName) == "" {
		return fmt.Errorf("KubernetesEngine network bridge name is empty")
	}
	if network.Spec.IpNetworkId == nil || strings.TrimSpace(*network.Spec.IpNetworkId) == "" {
		return fmt.Errorf("KubernetesEngine network ipNetworkId is empty")
	}

	resolvedVersion, binaries, err := resolveControlPlaneBinaries(mkeConf, ke)
	if err != nil {
		return err
	}
	ipAddress, maskBits, apiServerPort, allocated, err := resolveControlPlaneAddress(database, network, ke)
	if err != nil {
		return err
	}
	if allocated {
		defer func() {
			if allocated && err != nil {
				_ = database.ReleaseIP(api.VirtualNetworkID(network), *network.Spec.IpNetworkId, ipAddress)
			}
		}()
	}
	hostIPAddress, hostAllocated, err := resolveControlPlaneHostAddress(database, network, ke)
	if err != nil {
		return err
	}
	if hostAllocated {
		defer func() {
			if hostAllocated && err != nil {
				_ = database.ReleaseIP(api.VirtualNetworkID(network), *network.Spec.IpNetworkId, hostIPAddress)
			}
		}()
	}
	if err = database.UpdateKubernetesEngineControlPlaneStatus(api.KubernetesEngineID(ke), ipAddress, hostIPAddress, apiServerPort, resolvedVersion); err != nil {
		return err
	}
	allocated = false
	hostAllocated = false

	networkCfg, err := NewKubernetesEngineControlPlaneNetworkConfig(clusterName, *network.Spec.BridgeName, fmt.Sprintf("%s/%d", ipAddress, maskBits))
	if err != nil {
		return err
	}
	networkCfg.HostCIDR = fmt.Sprintf("%s/%d", hostIPAddress, maskBits)
	// Only rollback network resources if we created them in this call.
	hadNamespace := controlPlaneNamespaceExists(networkCfg.Namespace)
	if err = SetupKubernetesEngineControlPlaneNetwork(networkCfg); err != nil {
		return err
	}
	if !hadNamespace {
		defer func() {
			if err != nil {
				_ = TeardownKubernetesEngineControlPlaneNetwork(networkCfg)
			}
		}()
	}
	if _, _, err = ProvisionKubernetesEngineEtcd(database, mkeConf, ownEtcdURL, ke); err != nil {
		return err
	}
	assets, err := EnsureKubernetesEngineControlPlaneAssets(DefaultKubernetesEnginePkiDir, DefaultKubernetesControlPlaneConfigDir, clusterName, ipAddress, apiServerPort, hostBindAddress)
	if err != nil {
		return err
	}
	etcdClientPort := 0
	if ke.Status != nil && ke.Status.EtcdClientPort != nil {
		etcdClientPort = *ke.Status.EtcdClientPort
	} else {
		updated, getErr := database.GetKubernetesEngineById(api.KubernetesEngineID(ke))
		if getErr != nil || updated.Status == nil || updated.Status.EtcdClientPort == nil {
			return fmt.Errorf("failed to load allocated etcd client port: %w", getErr)
		}
		etcdClientPort = *updated.Status.EtcdClientPort
	}
	unitCfg := KubernetesEngineControlPlaneUnitConfig{
		ClusterName:        clusterName,
		NetworkNamespace:   networkCfg.Namespace,
		APIServerIP:        ipAddress,
		APIServerPort:      apiServerPort,
		EtcdClientPort:     etcdClientPort,
		Binaries:           binaries,
		Assets:             assets,
		ServiceClusterCIDR: DefaultKubernetesServiceClusterCIDR,
	}
	if err = CreateKubernetesEngineControlPlaneUnits(unitCfg); err != nil {
		return err
	}
	if err = EnsureKubernetesEngineControlPlaneNAT(hostBindAddress, ipAddress, apiServerPort); err != nil {
		return err
	}
	return CheckKubernetesEngineControlPlaneHealth(networkCfg.Namespace, assets.CACertPath, ipAddress, apiServerPort)
}

func DeprovisionKubernetesEngineControlPlane(database *db.Database, mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) error {
	clusterName, err := validateKubernetesEngineEtcdClusterName(ke.Metadata.Name)
	if err != nil {
		return err
	}
	if err := DeleteKubernetesEngineControlPlaneUnits(clusterName); err != nil {
		return err
	}
	if err := DeprovisionKubernetesEngineEtcd(clusterName); err != nil {
		return err
	}
	network, err := database.GetVirtualNetworkByName(kubernetesEngineNetworkName(ke))
	if err != nil {
		return err
	}
	if network.Spec.BridgeName == nil || network.Spec.IpNetworkId == nil || ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil {
		return fmt.Errorf("KubernetesEngine control plane network status is incomplete")
	}
	hostBindAddress := strings.TrimSpace(mkeConf.ControlPlaneBindAddress)
	if hostBindAddress != "" && ke.Status.ApiServerPort != nil {
		if err := RemoveKubernetesEngineControlPlaneNAT(hostBindAddress, *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort); err != nil {
			return err
		}
	}
	networkCfg, err := NewKubernetesEngineControlPlaneNetworkConfig(clusterName, *network.Spec.BridgeName, *ke.Status.ControlPlaneIpAddress+"/32")
	if err != nil {
		return err
	}
	if err := TeardownKubernetesEngineControlPlaneNetwork(networkCfg); err != nil {
		return err
	}
	if err := database.ReleaseIP(api.VirtualNetworkID(network), *network.Spec.IpNetworkId, *ke.Status.ControlPlaneIpAddress); err != nil {
		return err
	}
	if ke.Status.ControlPlaneHostIpAddress != nil {
		return database.ReleaseIP(api.VirtualNetworkID(network), *network.Spec.IpNetworkId, *ke.Status.ControlPlaneHostIpAddress)
	}
	return nil
}

func resolveControlPlaneBinaries(mkeConf *marmotd.MKEConfig, ke api.KubernetesEngine) (string, map[string]string, error) {
	if ke.Status != nil && ke.Status.ResolvedKubernetesVersion != nil && strings.TrimSpace(*ke.Status.ResolvedKubernetesVersion) != "" {
		resolved := strings.TrimSpace(*ke.Status.ResolvedKubernetesVersion)
		binaries, err := EnsureResolvedKubernetesControlPlaneBinaries(DefaultKubernetesControlPlaneBinaryDir, resolved)
		return resolved, binaries, err
	}
	return EnsureKubernetesControlPlaneBinaries(DefaultKubernetesControlPlaneBinaryDir, mkeConf.KubernetesVersion)
}

func resolveControlPlaneAddress(database *db.Database, network api.VirtualNetwork, ke api.KubernetesEngine) (ipAddress string, maskBits, apiServerPort int, allocated bool, err error) {
	ipNetworkID := strings.TrimSpace(*network.Spec.IpNetworkId)
	if ke.Status != nil && ke.Status.ControlPlaneIpAddress != nil && ke.Status.ApiServerPort != nil {
		ipNetwork, getErr := database.GetIpNetworkById(api.VirtualNetworkID(network), ipNetworkID)
		if getErr != nil {
			return "", 0, 0, false, getErr
		}
		if ipNetwork.AddressMaskLen == nil {
			return "", 0, 0, false, fmt.Errorf("IP network addressMaskLen is empty")
		}
		_, parsedNetwork, parseErr := net.ParseCIDR(*ipNetwork.AddressMaskLen)
		if parseErr != nil {
			return "", 0, 0, false, parseErr
		}
		maskBits, _ = parsedNetwork.Mask.Size()
		return *ke.Status.ControlPlaneIpAddress, maskBits, *ke.Status.ApiServerPort, false, nil
	}
	ipAddress, maskBits, err = database.AllocateIP(api.VirtualNetworkID(network), ipNetworkID, "mke-control-plane-"+api.KubernetesEngineID(ke))
	if err != nil {
		return "", 0, 0, false, err
	}
	engines, err := database.GetKubernetesEngines()
	if err != nil {
		_ = database.ReleaseIP(api.VirtualNetworkID(network), ipNetworkID, ipAddress)
		return "", 0, 0, false, err
	}
	apiServerPort, err = AllocateKubernetesEngineAPIServerPort(engines, DefaultKubernetesAPIServerPortRangeStart, DefaultKubernetesAPIServerPortRangeEnd)
	if err != nil {
		_ = database.ReleaseIP(api.VirtualNetworkID(network), ipNetworkID, ipAddress)
		return "", 0, 0, false, err
	}
	return ipAddress, maskBits, apiServerPort, true, nil
}

// resolveControlPlaneHostAddress は hostVeth (ルートnetns側) に割り当てるIPアドレスを解決する。
// 既にStatusに記録済みの場合はそれを再利用し(冪等)、未割り当ての場合は同じサブネットから
// 新規に採番する。このアドレスは、ルートnetnsからコントロールプレーンnetnsへの経路確立
// (kube-apiserverへのDNAT転送)専用であり、外部には公開しない。
func resolveControlPlaneHostAddress(database *db.Database, network api.VirtualNetwork, ke api.KubernetesEngine) (hostIPAddress string, allocated bool, err error) {
	if ke.Status != nil && ke.Status.ControlPlaneHostIpAddress != nil {
		return *ke.Status.ControlPlaneHostIpAddress, false, nil
	}
	ipNetworkID := strings.TrimSpace(*network.Spec.IpNetworkId)
	hostIPAddress, _, err = database.AllocateIP(api.VirtualNetworkID(network), ipNetworkID, "mke-control-plane-host-"+api.KubernetesEngineID(ke))
	if err != nil {
		return "", false, err
	}
	return hostIPAddress, true, nil
}
