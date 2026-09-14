package marmotd

import (
	"fmt"
	"log/slog"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/networkfabric"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
)

// ManagementNetworkName はマネジメント専用ネットワークの予約名(issue #696)。
const ManagementNetworkName = "mgmt"

// ManagementNetworkCIDR はマネジメント専用ネットワークの固定IPネット(issue #696)。
const ManagementNetworkCIDR = "10.245.0.0/16"

// ManagementNetworkHostAddress は、Marmotホスト自身がmgmtネットワーク上で使用する
// 固定IP(issue #696)。AllocateIP()はネットワークアドレス+1(ゲートウェイ相当)を
// 予約領域として自動的にスキップするため、ゲストVMへのIPAM払い出しと衝突しない。
const ManagementNetworkHostAddress = "10.245.0.1/16"

// ManagementNetworkACLAllowPriority / ManagementNetworkACLDenyPriority は
// mgmtネットワークのOVN ACL優先度(issue #696)。数値が大きいほど優先されるため、
// 許可リストを拒否ルールより高い優先度にすることで例外的に通信を許可する。
const (
	ManagementNetworkACLAllowPriority = 2000
	ManagementNetworkACLDenyPriority  = 1000
)

// EnsureManagementNetwork は予約名 "mgmt" の仮想ネットワークが存在しなければ作成する。
// etcdはクラスタで共有されるため、既に他ノードが作成済みであれば何もしない(冪等)。
// geneveオーバーレイで作成することで、クラスタ横断のOVN論理スイッチとして疎通できるようにする。
func (m *Marmot) EnsureManagementNetwork() error {
	if m == nil || m.Db == nil {
		return nil
	}

	if existing, err := m.Db.GetVirtualNetworkByName(ManagementNetworkName); err == nil {
		if existing.Spec.IpNetworkId == nil {
			// libvirt上に残存するmgmtがetcd再作成時にIPAM未紐付けのまま自動インポートされたケースを修復する(issue #696)。
			// 実際のIPAM作成/libvirt反映はDeployVirtualNetwork()に委ねるため、ここではPENDINGへ戻すのみ行う。
			return m.resetManagementNetworkForReprovisioning(existing)
		}
		return nil
	} else if err != db.ErrNotFound {
		return err
	}

	labels := map[string]interface{}{}
	db.SetNetworkSyncLabels(labels, "head", "", m.NodeName)
	// OVN ACLを実トラフィックへ適用するため、ゲストNICをOVN論理ポートとして正式に束縛する(issue #696)。
	labels[api.NetworkLabelACLEnforced] = "true"

	network := api.VirtualNetwork{
		ApiVersion: "v1",
		Kind:       "VirtualNetwork",
		Metadata: api.Metadata{
			Name:   ManagementNetworkName,
			Labels: &labels,
		},
		Spec: api.VirtualNetworkSpec{
			IPNetworkAddress: util.StringPtr(ManagementNetworkCIDR),
			// ゲストNICをOVN統合ブリッジへ直接接続し、OVN ACLを実効化する(issue #696)。
			BridgeName: util.StringPtr(api.OVNIntegrationBridgeName),
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

// resetManagementNetworkForReprovisioning は、libvirt上に残存していたmgmtネットワークが
// GetVirtualNetworksAndPutDB()によってIpNetworkId未設定のままACTIVEでetcdにインポートされた
// ケースを検知し、状態をPENDINGへ戻す(issue #696)。実際のIPネットワーク作成とlibvirt反映は
// 通常のプロビジョニング経路(reconcileHeadProvisioningNetwork / DeployVirtualNetwork)に委ねる。
func (m *Marmot) resetManagementNetworkForReprovisioning(vnet api.VirtualNetwork) error {
	vnetID := api.VirtualNetworkID(vnet)
	slog.Warn("management network found without IpNetworkId; resetting to PENDING for reprovisioning", "name", ManagementNetworkName, "id", vnetID)

	if vnet.Spec.IPNetworkAddress == nil {
		vnet.Spec.IPNetworkAddress = util.StringPtr(ManagementNetworkCIDR)
		if err := m.Db.UpdateVirtualNetworkById(vnetID, vnet); err != nil {
			return fmt.Errorf("failed to prepare management network for reprovisioning: %w", err)
		}
	}
	m.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_PENDING)
	return nil
}

// attachManagementNetworkInterface は、マニフェストの指定有無に関わらず、
// 予約名 "mgmt" ネットワーク用のNICを1本強制的に追加する(issue #696)。
// 既にマニフェストで "mgmt" が明示指定されている場合は何もしない。
// 他のNICのインデックス(PCIバス番号/ゲストOS側のインターフェース名対応)を変えないよう、
// 常に配列の末尾に追加する。
func (m *Marmot) attachManagementNetworkInterface(serverConfig *api.Server, virtSpec *virt.ServerSpec) error {
	if serverConfig.Spec.NetworkInterface != nil {
		for _, nic := range *serverConfig.Spec.NetworkInterface {
			if strings.TrimSpace(nic.Networkname) == ManagementNetworkName {
				return nil
			}
		}
	}

	vnet, err := m.Db.GetVirtualNetworkByName(ManagementNetworkName)
	if err != nil {
		slog.Error("GetVirtualNetworkByName(mgmt)", "err", err)
		return fmt.Errorf("management network '%s' is not found", ManagementNetworkName)
	}
	if vnet.Spec.IpNetworkId == nil {
		// ネットワークコントローラーのIPAM初期化が未完了。呼び出し元での再試行に委ねる。
		return fmt.Errorf("management network '%s' is not ready for IP allocation", ManagementNetworkName)
	}

	mac, err := util.GenerateRandomMAC()
	if err != nil {
		slog.Error("GenerateRandomMAC()", "err", err)
		return err
	}

	ipaddr, bitmask, err := m.Db.AllocateIP(api.VirtualNetworkID(vnet), *vnet.Spec.IpNetworkId, serverConfig.Metadata.Name)
	if err != nil {
		slog.Error("AllocateIP(mgmt)", "err", err)
		return err
	}
	ipnet, err := m.Db.GetIpNetworkById(api.VirtualNetworkID(vnet), *vnet.Spec.IpNetworkId)
	if err != nil {
		slog.Error("GetIpNetworkById(mgmt)", "err", err)
		return err
	}
	if err := m.Db.PutDnsEntry(serverConfig.Metadata.Name, ManagementNetworkName, ipaddr); err != nil {
		slog.Error("PutDnsEntry(mgmt)", "err", err)
		return err
	}

	existing := 0
	if serverConfig.Spec.NetworkInterface != nil {
		existing = len(*serverConfig.Spec.NetworkInterface)
	}
	busno := uint(existing + 1)
	if busno >= 3 {
		busno += 4 // diskとバス番号が被らないようにする
	}

	ns := virt.NetSpec{
		MAC:     mac.String(),
		Network: vnet.Metadata.Name,
		PortID:  uuid.New().String(),
		Bus:     busno,
	}
	if vnet.Spec.BridgeName != nil && shouldAttachOVSInterfaceID(vnet, strings.TrimSpace(*vnet.Spec.BridgeName)) {
		ns.InterfaceID = ns.PortID
	}
	// OVN ACLを実トラフィックへ適用するため、ゲストNICをOVN論理ポートとして正式に束縛する(issue #696)。
	if networkfabric.IsACLEnforcedNetwork(&vnet) {
		if err := networkfabric.NewOVNFabric().EnsureGuestLogicalPort(&vnet, ns.PortID, ns.MAC, ipaddr); err != nil {
			slog.Error("EnsureGuestLogicalPort(mgmt)", "err", err)
			return err
		}
	}
	virtSpec.NetSpecs = append(virtSpec.NetSpecs, ns)

	ni := api.NetworkInterface{
		Networkname: vnet.Metadata.Name,
		Networkid:   api.VirtualNetworkID(vnet),
		Mac:         &ns.MAC,
		Address:     util.StringPtr(ipaddr),
		IpNetworkId: util.StringPtr(*vnet.Spec.IpNetworkId),
	}
	if networkfabric.IsACLEnforcedNetwork(&vnet) {
		ni.InterfaceId = util.StringPtr(ns.PortID)
	}
	if bitmask > 0 {
		ni.Netmasklen = util.IntPtrInt(bitmask)
	} else if ipnet.Netmasklen != nil {
		ni.Netmasklen = util.IntPtrInt(*ipnet.Netmasklen)
	}
	if ipnet.Netmask != nil {
		ni.Netmask = util.StringPtr(*ipnet.Netmask)
	}

	if serverConfig.Spec.NetworkInterface == nil {
		serverConfig.Spec.NetworkInterface = &[]api.NetworkInterface{ni}
	} else {
		updated := append(*serverConfig.Spec.NetworkInterface, ni)
		serverConfig.Spec.NetworkInterface = &updated
	}

	return nil
}

// BuildManagementNetworkACLRules は marmotd.json の許可リスト(management_network_acl_allow)から、
// mgmtネットワークに適用するOVN ACLルール一覧を組み立てる(issue #696)。
// 許可リストに一致する通信は allow-related、それ以外の発信通信はすべて drop する。
func BuildManagementNetworkACLRules(cfg *MarmotdConfig) []networkfabric.ACLRule {
	rules := make([]networkfabric.ACLRule, 0)

	if cfg != nil {
		for _, entry := range cfg.ManagementNetworkACLAllow {
			match, ok := managementNetworkACLAllowMatch(entry)
			if !ok {
				slog.Warn("skip invalid management_network_acl_allow entry", "description", entry.Description, "cidr", entry.CIDR, "protocol", entry.Protocol, "port", entry.Port)
				continue
			}
			rules = append(rules, networkfabric.ACLRule{
				Direction: "from-lport",
				Priority:  ManagementNetworkACLAllowPriority,
				Match:     match,
				Action:    "allow-related",
			})
		}
	}

	// 許可リストに一致しない発信通信はすべて拒否する(ゲストVM同士の通信を含む)
	rules = append(rules, networkfabric.ACLRule{
		Direction: "from-lport",
		Priority:  ManagementNetworkACLDenyPriority,
		Match:     "ip4",
		Action:    "drop",
	})

	return rules
}

func managementNetworkACLAllowMatch(entry ManagementNetworkACLAllowEntry) (string, bool) {
	cidr := normalizeManagementNetworkACLCIDR(entry.CIDR)
	if cidr == "" {
		return "", false
	}
	proto := strings.ToLower(strings.TrimSpace(entry.Protocol))
	if proto != "tcp" && proto != "udp" {
		return "", false
	}
	if entry.Port <= 0 || entry.Port > 65535 {
		return "", false
	}
	return fmt.Sprintf("ip4 && ip4.dst==%s && %s.dst==%d", cidr, proto, entry.Port), true
}

func normalizeManagementNetworkACLCIDR(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if _, err := netip.ParsePrefix(trimmed); err == nil {
		return trimmed
	}
	if addr, err := netip.ParseAddr(trimmed); err == nil {
		return addr.String() + "/32"
	}
	return ""
}
