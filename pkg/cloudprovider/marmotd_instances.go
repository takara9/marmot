package cloudprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
	"github.com/takara9/marmot/pkg/db"
)

const (
	// kubernetesEngineNodeLabelOwner/Role/RoleValue は pkg/controller の同名ラベルと同じ文字列値。
	// KubernetesEngineのノード用仮想サーバーを識別するために使用する。
	kubernetesEngineNodeLabelOwner = "kubernetesEngineId"
	kubernetesEngineNodeLabelRole  = "kubernetesEngineRole"
	kubernetesEngineNodeRoleValue  = "node"
)

// MarmotdInstances は marmotd の Server API を参照して Instances を実装する。
type MarmotdInstances struct {
	endpoint            *client.MarmotEndpoint
	clusterID           string
	internalNetworkName string
	externalNetworkName string
	region              string
	zone                string
}

// NewMarmotdInstances は、clusterID が所有するノードのみを対象に、internalNetworkName
// (ノード間通信用ネットワーク)をInternalIP、externalNetworkName(通常はhost-bridge。未使用時は
// 空文字列)をExternalIPとして扱う MarmotdInstances を組み立てる。region/zoneは、mke.json由来の
// 値をInstanceMetadataにそのまま設定する(空文字列であれば設定しない)。
func NewMarmotdInstances(endpoint *client.MarmotEndpoint, clusterID, internalNetworkName, externalNetworkName, region, zone string) *MarmotdInstances {
	return &MarmotdInstances{
		endpoint:            endpoint,
		clusterID:           clusterID,
		internalNetworkName: internalNetworkName,
		externalNetworkName: externalNetworkName,
		region:              region,
		zone:                zone,
	}
}

var _ Instances = (*MarmotdInstances)(nil)

func (i *MarmotdInstances) InstanceExists(ctx context.Context, nodeName string) (bool, error) {
	server, err := i.findNodeServer(ctx, nodeName)
	if err != nil {
		return false, err
	}
	return server != nil, nil
}

func (i *MarmotdInstances) InstanceShutdown(ctx context.Context, nodeName string) (bool, error) {
	server, err := i.findNodeServer(ctx, nodeName)
	if err != nil {
		return false, err
	}
	if server == nil {
		return false, fmt.Errorf("cloudprovider: no server found for node %q in cluster %q", nodeName, i.clusterID)
	}
	return server.Status != nil && server.Status.StatusCode == db.SERVER_STOPPED, nil
}

func (i *MarmotdInstances) InstanceMetadata(ctx context.Context, nodeName string) (*InstanceMetadata, error) {
	server, err := i.findNodeServer(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, fmt.Errorf("cloudprovider: no server found for node %q in cluster %q", nodeName, i.clusterID)
	}

	addresses := []NodeAddress{{Type: NodeHostName, Address: server.Metadata.Name}}
	if addr, ok := serverNetworkAddress(*server, i.internalNetworkName); ok {
		addresses = append(addresses, NodeAddress{Type: NodeInternalIP, Address: addr})
	}
	if strings.TrimSpace(i.externalNetworkName) != "" {
		if addr, ok := serverNetworkAddress(*server, i.externalNetworkName); ok {
			addresses = append(addresses, NodeAddress{Type: NodeExternalIP, Address: addr})
		}
	}

	instanceType := ""
	if server.Spec.Cpu != nil && server.Spec.Memory != nil {
		instanceType = fmt.Sprintf("%dvcpu-%dmi", *server.Spec.Cpu, *server.Spec.Memory)
	}

	return &InstanceMetadata{
		ProviderID:    fmt.Sprintf("marmot://%s/%s", i.clusterID, server.Metadata.Id),
		InstanceType:  instanceType,
		NodeAddresses: addresses,
		Region:        i.region,
		Zone:          i.zone,
	}, nil
}

// findNodeServer は marmotd から Server 一覧を取得し、clusterID が所有する node ロールの
// うち nodeName に一致するものを返す。見つからない場合は (nil, nil) を返す。
func (i *MarmotdInstances) findNodeServer(ctx context.Context, nodeName string) (*api.Server, error) {
	body, _, err := i.endpoint.GetServers()
	if err != nil {
		return nil, fmt.Errorf("cloudprovider: failed to fetch servers from marmotd: %w", err)
	}
	var servers []api.Server
	if err := json.Unmarshal(body, &servers); err != nil {
		return nil, fmt.Errorf("cloudprovider: failed to decode marmotd servers response: %w", err)
	}
	for idx := range servers {
		server := servers[idx]
		if server.Metadata.Name != nodeName {
			continue
		}
		if server.Metadata.Labels == nil {
			continue
		}
		labels := *server.Metadata.Labels
		if labels[kubernetesEngineNodeLabelOwner] != i.clusterID {
			continue
		}
		if labels[kubernetesEngineNodeLabelRole] != kubernetesEngineNodeRoleValue {
			continue
		}
		return &server, nil
	}
	return nil, nil
}

func serverNetworkAddress(server api.Server, networkName string) (string, bool) {
	if strings.TrimSpace(networkName) == "" || server.Spec.NetworkInterface == nil {
		return "", false
	}
	for _, nic := range *server.Spec.NetworkInterface {
		if nic.Networkname == networkName && nic.Address != nil && strings.TrimSpace(*nic.Address) != "" {
			return strings.TrimSpace(*nic.Address), true
		}
	}
	return "", false
}
