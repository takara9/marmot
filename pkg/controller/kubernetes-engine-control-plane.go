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
	if err = database.UpdateKubernetesEngineControlPlaneStatus(api.KubernetesEngineID(ke), ipAddress, apiServerPort, resolvedVersion); err != nil {
		return err
	}
	allocated = false

	networkCfg, err := NewKubernetesEngineControlPlaneNetworkConfig(clusterName, *network.Spec.BridgeName, fmt.Sprintf("%s/%d", ipAddress, maskBits))
	if err != nil {
		return err
	}
	if err = SetupKubernetesEngineControlPlaneNetwork(networkCfg); err != nil {
		return err
	}
	if _, _, err = ProvisionKubernetesEngineEtcd(database, mkeConf, ownEtcdURL, ke); err != nil {
		_ = TeardownKubernetesEngineControlPlaneNetwork(networkCfg)
		return err
	}
	assets, err := EnsureKubernetesEngineControlPlaneAssets(DefaultKubernetesEnginePkiDir, DefaultKubernetesControlPlaneConfigDir, clusterName, ipAddress, apiServerPort)
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
	return CheckKubernetesEngineControlPlaneHealth(networkCfg.Namespace, assets.CACertPath, ipAddress, apiServerPort)
}

func DeprovisionKubernetesEngineControlPlane(database *db.Database, ke api.KubernetesEngine) error {
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
	networkCfg, err := NewKubernetesEngineControlPlaneNetworkConfig(clusterName, *network.Spec.BridgeName, *ke.Status.ControlPlaneIpAddress+"/32")
	if err != nil {
		return err
	}
	if err := TeardownKubernetesEngineControlPlaneNetwork(networkCfg); err != nil {
		return err
	}
	return database.ReleaseIP(api.VirtualNetworkID(network), *network.Spec.IpNetworkId, *ke.Status.ControlPlaneIpAddress)
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
