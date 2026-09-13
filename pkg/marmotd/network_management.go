package marmotd

import (
	"log/slog"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

// ManagementNetworkName はマネジメント専用ネットワークの予約名(issue #696)。
const ManagementNetworkName = "mgmt"

// ManagementNetworkCIDR はマネジメント専用ネットワークの固定IPネット(issue #696)。
const ManagementNetworkCIDR = "10.245.0.0/16"

// EnsureManagementNetwork は予約名 "mgmt" の仮想ネットワークが存在しなければ作成する。
// etcdはクラスタで共有されるため、既に他ノードが作成済みであれば何もしない(冪等)。
// geneveオーバーレイで作成することで、クラスタ横断のOVN論理スイッチとして疎通できるようにする。
func (m *Marmot) EnsureManagementNetwork() error {
	if m == nil || m.Db == nil {
		return nil
	}

	if _, err := m.Db.GetVirtualNetworkByName(ManagementNetworkName); err == nil {
		return nil
	} else if err != db.ErrNotFound {
		return err
	}

	labels := map[string]interface{}{}
	db.SetNetworkSyncLabels(labels, "head", "", m.NodeName)

	network := api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata: api.Metadata{
			Name:   ManagementNetworkName,
			Labels: &labels,
		},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr(ManagementNetworkCIDR),
		},
	}
	if strings.TrimSpace(m.NodeName) != "" {
		network.Metadata.NodeName = util.StringPtr(m.NodeName)
	}

	if err := applyVirtualNetworkDefaults(&network, CurrentConfig(), m.Db); err != nil {
		return err
	}

	if _, err := m.Db.CreateVirtualNetwork(network); err != nil {
		// 他ノードが並行して作成した場合はエラーにしない
		if strings.Contains(err.Error(), "already exists") {
			slog.Debug("management network already created by another node", "name", ManagementNetworkName)
			return nil
		}
		return err
	}

	slog.Debug("management network created", "name", ManagementNetworkName, "cidr", ManagementNetworkCIDR)
	return nil
}
