package marmotd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

func shouldPreserveSystemNetworkCreationTimestamp(name string) bool {
	switch strings.TrimSpace(name) {
	case "default", "host-bridge":
		return true
	default:
		return false
	}
}

func mergeImportedNetworkPreservingCreation(existing api.VirtualNetwork, imported api.VirtualNetwork, nodeName string, now time.Time) api.VirtualNetwork {
	merged := existing
	merged.ApiVersion = imported.ApiVersion
	merged.Kind = imported.Kind
	merged.Metadata.Name = imported.Metadata.Name
	if strings.TrimSpace(nodeName) != "" {
		merged.Metadata.NodeName = util.StringPtr(strings.TrimSpace(nodeName))
	}
	merged.Spec = imported.Spec
	if merged.Status == nil {
		merged.Status = &api.Status{}
	}
	if merged.Status.CreationTimeStamp == nil {
		merged.Status.CreationTimeStamp = util.TimePtr(now)
	}
	merged.Status.StatusCode = db.NETWORK_ACTIVE
	merged.Status.Status = util.StringPtr(db.NetworkStatus[db.NETWORK_ACTIVE])
	merged.Status.LastUpdateTimeStamp = util.TimePtr(now)
	merged.Status.Message = nil
	return merged
}

func shouldPreserveDistributedNetworkEntry(existing api.VirtualNetwork) bool {
	if existing.Metadata.Labels == nil {
		return false
	}
	labels := *existing.Metadata.Labels
	role := strings.TrimSpace(db.GetNetworkSyncRole(labels))
	if role == "head" || role == "follower" {
		return true
	}
	return strings.TrimSpace(db.GetHeadNetworkID(labels)) != ""
}

func findNetworkByNameAndNode(networks []api.VirtualNetwork, name string, nodeName string) (api.VirtualNetwork, bool) {
	targetName := strings.TrimSpace(name)
	targetNode := strings.TrimSpace(nodeName)
	for _, network := range networks {
		if strings.TrimSpace(network.Metadata.Name) != targetName {
			continue
		}
		if targetNode == "" {
			return network, true
		}
		if network.Metadata.NodeName == nil {
			continue
		}
		if strings.TrimSpace(*network.Metadata.NodeName) == targetNode {
			return network, true
		}
	}
	return api.VirtualNetwork{}, false
}

// 起動時に既存ネットワークを取得して、データベースへの登録
func (m *Marmot) GetVirtualNetworksAndPutDB() ([]api.VirtualNetwork, error) {
	slog.Debug("GetVirtualNetworks called")
	var apiNetworks []api.VirtualNetwork

	// 実態ネットワークを取得
	networks, err := m.Virt.GetVirtualNetworks()
	if err != nil {
		slog.Error("Failed to get virtual networks from libvirt", "err", err)
		return nil, err
	}

	// 取得した実態ネットワークをETCDに登録
	for _, n := range *networks {
		var net api.VirtualNetwork
		var meta api.Metadata
		var spec api.VirtualNetworkSpec
		net.Metadata = meta
		net.Spec = spec

		net.ApiVersion = "v1"
		net.Kind = "VirtualNetwork"

		// name
		name, err := n.GetName()
		if err != nil {
			slog.Error("Failed to get virtual network name", "err", err)
			continue
		}
		net.Metadata.Name = name

		// uuid
		uuid, err := n.GetUUIDString()
		if err != nil {
			slog.Error("Failed to get virtual network UUID", "err", err)
			continue
		}
		net.Metadata.Uuid = util.StringPtr(uuid)
		api.SetVirtualNetworkID(&net, uuid[:5]) // IDはUUIDの先頭8文字を使用

		// Bridge
		bridge, err := n.GetBridgeName()
		if err != nil {
			slog.Error("Failed to get virtual network bridge name", "err", err)
			continue
		}
		net.Spec.BridgeName = util.StringPtr(bridge)

		xml, err := n.GetXMLDesc(0)
		if err != nil {
			slog.Error("Failed to get virtual network XML", "err", err)
			continue
		}
		debugPrintln(string(xml))

		// libvirt XMLをパースして、APIのVirtualNetworkに変換る
		var libnet libvirtxml.Network
		err = libnet.Unmarshal(xml)
		if err != nil {
			return nil, err
		}

		if libnet.Forward != nil {
			net.Spec.ForwardMode = util.StringPtr(libnet.Forward.Mode)
			net.Spec.Nat = util.BoolPtr(libnet.Forward.NAT != nil)
		}
		if libnet.MAC != nil {
			net.Spec.MacAddress = util.StringPtr(libnet.MAC.Address)
		}
		if len(libnet.IPs) > 0 {
			net.Spec.IpAddress = util.StringPtr(libnet.IPs[0].Address)
			net.Spec.Netmask = util.StringPtr(libnet.IPs[0].Netmask)
			if libnet.IPs[0].DHCP != nil && len(libnet.IPs[0].DHCP.Ranges) > 0 {
				net.Spec.Dhcp = util.BoolPtr(true)
				net.Spec.DhcpStartAddress = util.StringPtr(libnet.IPs[0].DHCP.Ranges[0].Start)
				net.Spec.DhcpEndAddress = util.StringPtr(libnet.IPs[0].DHCP.Ranges[0].End)
			}
		}

		// TODO: VLAN Trunk を追加

		// 同じ名前・同じノードのネットワークが既にETCDに登録されているか確認
		networksInDB, err := m.Db.GetVirtualNetworks()
		if err != nil {
			slog.Error("Failed to list virtual networks from ETCD", "err", err)
			continue
		}
		existingNet, found := findNetworkByNameAndNode(networksInDB, name, m.NodeName)
		if found {
			if networkDeletionRequested(existingNet) {
				slog.Debug("Skipping libvirt network import because deletion is already requested for same-name network", "networkName", name, "existingNetworkId", api.VirtualNetworkID(existingNet))
				continue
			}
			existingUUID := strings.TrimSpace(util.OrDefault(existingNet.Metadata.Uuid, ""))
			importedUUID := strings.TrimSpace(util.OrDefault(net.Metadata.Uuid, ""))
			if existingUUID != importedUUID {
				if shouldPreserveDistributedNetworkEntry(existingNet) {
					merged := mergeImportedNetworkPreservingCreation(existingNet, net, m.NodeName, time.Now())
					err := m.Db.UpdateVirtualNetworkById(api.VirtualNetworkID(existingNet), merged)
					if err != nil {
						slog.Error("Failed to preserve distributed network entry during import", "err", err, "networkName", name, "networkId", api.VirtualNetworkID(existingNet))
						continue
					}
					apiNetworks = append(apiNetworks, merged)
					continue
				}
				if shouldPreserveSystemNetworkCreationTimestamp(name) && existingNet.Status != nil && existingNet.Status.CreationTimeStamp != nil {
					merged := mergeImportedNetworkPreservingCreation(existingNet, net, m.NodeName, time.Now())
					err := m.Db.UpdateVirtualNetworkById(api.VirtualNetworkID(existingNet), merged)
					if err != nil {
						slog.Error("Failed to preserve system network creation timestamp", "err", err, "networkName", name, "networkId", api.VirtualNetworkID(existingNet))
						continue
					}
					apiNetworks = append(apiNetworks, merged)
					continue
				}

				err := m.Db.DeleteVirtualNetworkById(api.VirtualNetworkID(existingNet))
				if err != nil {
					slog.Error("Failed to delete existing virtual network in ETCD", "err", err, "networkId", api.VirtualNetworkID(existingNet))
					continue
				}
			}
		}

		// 既にETCDに登録されているか確認
		_, err = m.Db.GetVirtualNetworkById(api.VirtualNetworkID(net))
		if err == nil {
			slog.Debug("Virtual network already exists in ETCD, skipping", "id", api.VirtualNetworkID(net))
			continue
		} else if err == db.ErrNotFound {
			// このノードで発見したネットワークにノード名を付与する
			if m != nil && m.NodeName != "" {
				net.Metadata.NodeName = util.StringPtr(m.NodeName)
			}
			// データベースに登録
			if err := m.Db.PutVirtualNetworksETCD(net); err != nil {
				slog.Error("Failed to put virtual network to ETCD", "err", err)
			}
		}

		// 仮想ネットワークの状態をACTIVEに更新
		//net.Status = &api.Status{
		//	Status: util.IntPtrInt(db.NETWORK_ACTIVE),
		//}
		m.Db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(net), db.NETWORK_ACTIVE)
		if strings.TrimSpace(net.Metadata.Name) == "host-bridge" {
			if err := m.ensureHostBridgeIPNetwork(&net); err != nil {
				slog.Error("Failed to sync host-bridge IP network from config", "err", err, "networkId", api.VirtualNetworkID(net))
				continue
			}
		}

		// 戻り値に追加
		apiNetworks = append(apiNetworks, net)
	}
	return apiNetworks, nil
}

func cloneRoutes(routes *[]api.Route) *[]api.Route {
	if routes == nil {
		return nil
	}
	out := make([]api.Route, 0, len(*routes))
	for _, r := range *routes {
		to := strings.TrimSpace(util.OrDefault(r.To, ""))
		via := strings.TrimSpace(util.OrDefault(r.Via, ""))
		if to == "" || via == "" {
			continue
		}
		out = append(out, api.Route{To: util.StringPtr(to), Via: util.StringPtr(via)})
	}
	return &out
}

func cloneNameservers(ns *api.Nameservers) *api.Nameservers {
	if ns == nil {
		return nil
	}
	copyNS := api.Nameservers{}
	if ns.Addresses != nil {
		addresses := make([]string, 0, len(*ns.Addresses))
		for _, addr := range *ns.Addresses {
			if v := strings.TrimSpace(addr); v != "" {
				addresses = append(addresses, v)
			}
		}
		copyNS.Addresses = &addresses
	}
	if ns.Search != nil {
		search := make([]string, 0, len(*ns.Search))
		for _, domain := range *ns.Search {
			if v := strings.TrimSpace(domain); v != "" {
				search = append(search, v)
			}
		}
		copyNS.Search = &search
	}
	return &copyNS
}

func (m *Marmot) ensureHostBridgeIPNetwork(vnet *api.VirtualNetwork) error {
	if m == nil || m.Db == nil || vnet == nil {
		return nil
	}
	if strings.TrimSpace(vnet.Metadata.Name) != "host-bridge" {
		return nil
	}

	cfg := CurrentConfig()
	if cfg == nil {
		return nil
	}
	netCIDR := strings.TrimSpace(cfg.HostBridgeIPNetAddr)
	start := strings.TrimSpace(cfg.HostBridgeIPAddrStart)
	end := strings.TrimSpace(cfg.HostBridgeIPAddrEnd)
	if netCIDR == "" || start == "" || end == "" {
		return nil
	}

	prefix, err := netip.ParsePrefix(netCIDR)
	if err != nil {
		return fmt.Errorf("invalid host-bridge-ip-net-addr: %w", err)
	}
	prefix = prefix.Masked()

	vnetID := api.VirtualNetworkID(*vnet)
	networks, err := m.Db.GetIpNetworks(vnetID)
	if err != nil {
		return err
	}

	targetID := ""
	if vnet.Spec.IpNetworkId != nil && strings.TrimSpace(*vnet.Spec.IpNetworkId) != "" {
		targetID = strings.TrimSpace(*vnet.Spec.IpNetworkId)
	} else {
		for _, ipn := range networks {
			if ipn.AddressMaskLen == nil {
				continue
			}
			if strings.TrimSpace(*ipn.AddressMaskLen) == prefix.String() {
				targetID = ipn.Id
				break
			}
		}
	}

	if targetID == "" {
		id, err := m.Db.CreateIpNetwork(vnetID, &api.IPNetwork{
			AddressMaskLen: util.StringPtr(prefix.String()),
			StartAddress:   util.StringPtr(start),
			EndAddress:     util.StringPtr(end),
		})
		if err != nil {
			return err
		}
		targetID = id
	}

	ipnet, err := m.Db.GetIpNetworkById(vnetID, targetID)
	if err != nil {
		return err
	}
	ipnet.AddressMaskLen = util.StringPtr(prefix.String())
	ipnet.StartAddress = util.StringPtr(start)
	ipnet.EndAddress = util.StringPtr(end)
	if cfg.HostBridgeDefault != nil {
		ipnet.Routes = cloneRoutes(cfg.HostBridgeDefault.Routes)
		ipnet.Nameservers = cloneNameservers(cfg.HostBridgeDefault.Nameservers)
		if cfg.HostBridgeDefault.Netmasklen != nil {
			ipnet.Netmasklen = util.IntPtrInt(*cfg.HostBridgeDefault.Netmasklen)
		}
	}
	key := db.NetworkPrefix + "/" + vnetID + "/ip_network/" + targetID
	if err := m.Db.PutJSON(key, *ipnet); err != nil {
		return err
	}

	vnet.Spec.IpNetworkId = util.StringPtr(targetID)
	if err := m.Db.UpdateVirtualNetworkById(vnetID, *vnet); err != nil {
		return err
	}

	return nil
}

// 仮想ネットワークの作成
// この関数は、PENDING状態のオブジェクトを受け取ることを想定している
func (m *Marmot) DeployVirtualNetwork(vnet api.VirtualNetwork) error {
	vnetID := api.VirtualNetworkID(vnet)
	slog.Debug("DeployVirtualNetwork called", "vnet", vnetID)

	// 仮想ネットワークの状態をPROVISIONINGに更新
	m.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_PROVISIONING)

	// 仮想ネットワークのXMLを作成
	xml, err := virt.CreateVirtualNetworkXML(vnet)
	if err != nil {
		slog.Error("Failed to create virtual network XML", "err", err)
		return err
	}
	slog.Debug("Virtual network XML created", "id", vnetID)

	// IPネットワークの指定があるかの判定
	if vnet.Spec.IPNetworkAddress == nil {
		// 任意のIPネットワーク作成
		id, err := m.Db.CreateAnyIpNetwork(vnetID)
		if err != nil {
			slog.Error("Failed to create IP network", "err", err)
			return err
		}
		vnet.Spec.IpNetworkId = util.StringPtr(id)
	} else {
		// 指定されたIPネットワーク作成
		ipNetworkSpec := api.IPNetwork{
			AddressMaskLen: vnet.Spec.IPNetworkAddress,
		}
		id, err := m.Db.CreateIpNetwork(vnetID, &ipNetworkSpec)
		if err != nil {
			slog.Error("Failed to create IP network", "err", err)
			return err
		}
		vnet.Spec.IpNetworkId = util.StringPtr(id)
	}

	// 仮想ネットワークの作成と起動
	if err := m.Virt.DefineAndStartVirtualNetwork(*xml); err != nil {
		slog.Error("Failed to define and start virtual network", "err", err)
		return err
	}

	//　仮想ネットワークのデータを更新
	if err := m.Db.UpdateVirtualNetworkById(vnetID, vnet); err != nil {
		slog.Error("Failed to update virtual network", "err", err)
		return err
	}

	// 仮想ネットワークの作成の成功したか確認は？
	// コントローラーにまかせるか？

	// 仮想ネットワークの状態をACTIVEに更新
	m.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ACTIVE)
	return nil
}

// 仮想ネットワークの削除
// この関数は、deletion timestampがセットされたオブジェクトを受け取ることを想定している
func (m *Marmot) DeleteVirtualNetwork(networkId string) error {
	slog.Debug("DeleteVirtualNetwork called", "networkId", networkId)

	// 仮想ネットワークの状態をDELETINGに更新
	m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_DELETING)

	vnet, err := m.Db.GetVirtualNetworkById(networkId)
	if err != nil {
		slog.Error("Failed to get virtual network for deletion", "err", err, "networkId", networkId)
		return err
	}

	debugPrintln("==== Deleting virtual network:", vnet.Metadata.Name, "====")
	jsonBytes, err := json.MarshalIndent(vnet, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal virtual network for deletion", "err", err)
	} else {
		debugPrintln(string(jsonBytes))
	}

	// 仮想ネットワークに関連付いているIPネットワークをチェックして、使用していれば消せないエラーを返す
	if vnet.Spec.IpNetworkId != nil {
		ipNetId := *vnet.Spec.IpNetworkId
		inUse, err := m.Db.CheckIPnetInUse(networkId, ipNetId)
		if err != nil {
			m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ACTIVE)
			slog.Error("Failed to check if IP network is in use", "err", err, "ipNetworkId", ipNetId)
			return err
		}
		if inUse {
			m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ACTIVE)
			slog.Warn("Cannot delete virtual network because associated IP network is still in use", "ipNetworkId", ipNetId)
			return fmt.Errorf("cannot delete virtual network because associated IP network %s is still in use", ipNetId)
		}
	}

	// 実態を先に消す
	// 仮想ネットワークの実態　削除処理を実装
	if err := m.Virt.DeleteVirtualNetwork(vnet.Metadata.Name); err != nil {
		m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ERROR)
		slog.Error("Failed to delete virtual network", "err", err)
		return err
	}

	// 削除 API が成功しても libvirt 実態が残っているケースを検出する。
	// 実態が残っている間は DB エントリーを消さず、次の制御ループで再試行させる。
	if _, found, err := m.Virt.GetVirtualNetworkByName(vnet.Metadata.Name); err != nil {
		m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ERROR)
		slog.Error("Failed to verify virtual network absence", "err", err, "networkName", vnet.Metadata.Name)
		return err
	} else if found {
		verifyErr := fmt.Errorf("virtual network %s still exists after delete request", vnet.Metadata.Name)
		m.Db.UpdateVirtualNetworkStatusWithMessage(networkId, db.NETWORK_ERROR, "libvirt:delete-verify-failed:"+verifyErr.Error())
		slog.Warn("Virtual network still present after delete request; will retry in next loop", "networkId", networkId, "networkName", vnet.Metadata.Name)
		return verifyErr
	}

	// 実態が消えたら、データベースからも削除する
	// DeleteVirtualNetworkById 内で必要に応じて紐付いたIPネットワークも削除する。
	if err := m.Db.DeleteVirtualNetworkById(api.VirtualNetworkID(vnet)); err != nil {
		m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ERROR)
		slog.Error("Failed to delete virtual network", "err", err)
		return err
	}

	return nil
}

// 仮想ネットワークの参照
func (m *Marmot) GetVirtualNetwork() ([]api.VirtualNetwork, error) {
	slog.Debug("GetVirtualNetwork called")
	networks, err := m.Db.GetVirtualNetworks()
	if err != nil {
		slog.Error("Failed to get virtual networks", "err", err)
		return nil, err
	}
	return networks, nil
}

// ETCDに登録されたオブジェクトと実態ネットワークを比較して、ETCDの状態を更新する
func (m *Marmot) CheckVirtualNetworks() error {
	slog.Debug("CheckVirtualNetworks called")

	// ETCDに登録されたオブジェクトと実態を比較して、ETCDの状態を更新する
	vNetworks, err := m.Db.GetVirtualNetworks()
	if err != nil {
		slog.Error("Failed to get virtual networks", "err", err)
		return err
	}
	for _, vnet := range vNetworks {
		vnetID := api.VirtualNetworkID(vnet)
		_, found, err := m.Virt.GetVirtualNetworkByName(vnet.Metadata.Name)
		if err != nil {
			slog.Error("Error checking virtual network existence", "err", err, "networkId", vnetID)
			if !found {
				// Createから１０分経過しても実態が存在しない場合は、エラーにする
				if vnet.Status != nil && vnet.Status.CreationTimeStamp != nil {
					creationTime := *vnet.Status.CreationTimeStamp
					if time.Since(creationTime) > 10*time.Minute {
						slog.Debug("仮想ネットワークの実態が存在しないため、エラーにする", "networkId", vnetID)
						m.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ERROR)
						continue
					}
				} else {
					slog.Debug("CreationTimeStamp is nil, skipping error update for now", "networkId", vnetID)
				}
			} else {
				// 削除予定が無いことを確認して、ACTIVEに更新する
				if vnet.Status != nil && vnet.Status.DeletionTimeStamp == nil {
					slog.Debug("仮想ネットワークの実態が存在、削除予定が無いためACTIVEに更新", "networkId", vnetID)
					m.Db.UpdateVirtualNetworkStatus(vnetID, db.NETWORK_ACTIVE)
					continue
				}
			}
		}
	}

	// 実態ネットワークを取得して、ETCDに登録されたオブジェクトと比較して、ETCDに登録されていない実態があれば、ETCDに登録する
	networks, err := m.Virt.GetVirtualNetworks()
	if err != nil {
		slog.Error("Failed to get virtual networks from libvirt", "err", err)
		return err
	}
	for _, n := range *networks {
		vnet, err := convertLibvirtNetworkToAPINetwork(n)
		if err != nil {
			slog.Error("Failed to convert libvirt network to API network", "err", err)
			continue
		}

		// 同じ名前・同じノードのネットワークが既にETCDに登録されている場合は、UUID差分を確認する。
		networksInDB, listErr := m.Db.GetVirtualNetworks()
		if listErr != nil {
			slog.Error("Failed to list virtual networks from ETCD", "err", listErr)
			continue
		}
		existingNet, found := findNetworkByNameAndNode(networksInDB, vnet.Metadata.Name, m.NodeName)
		if found {
			if networkDeletionRequested(existingNet) {
				slog.Debug("Skipping libvirt network import because deletion is already requested for same-name network", "networkName", vnet.Metadata.Name, "existingNetworkId", api.VirtualNetworkID(existingNet))
				continue
			}

			existingUUID := strings.TrimSpace(util.OrDefault(existingNet.Metadata.Uuid, ""))
			importedUUID := strings.TrimSpace(util.OrDefault(vnet.Metadata.Uuid, ""))
			if existingUUID != importedUUID {
				if shouldPreserveDistributedNetworkEntry(existingNet) {
					merged := mergeImportedNetworkPreservingCreation(existingNet, *vnet, m.NodeName, time.Now())
					if err := m.Db.UpdateVirtualNetworkById(api.VirtualNetworkID(existingNet), merged); err != nil {
						slog.Error("Failed to preserve distributed network entry during sync", "err", err, "networkName", vnet.Metadata.Name, "networkId", api.VirtualNetworkID(existingNet))
						continue
					}
					m.Db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(existingNet), db.NETWORK_ACTIVE)
					continue
				}
				if shouldPreserveSystemNetworkCreationTimestamp(vnet.Metadata.Name) && existingNet.Status != nil && existingNet.Status.CreationTimeStamp != nil {
					merged := mergeImportedNetworkPreservingCreation(existingNet, *vnet, m.NodeName, time.Now())
					if err := m.Db.UpdateVirtualNetworkById(api.VirtualNetworkID(existingNet), merged); err != nil {
						slog.Error("Failed to preserve system network creation timestamp", "err", err, "networkName", vnet.Metadata.Name, "networkId", api.VirtualNetworkID(existingNet))
						continue
					}
					m.Db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(existingNet), db.NETWORK_ACTIVE)
					continue
				}

				if err := m.Db.DeleteVirtualNetworkById(api.VirtualNetworkID(existingNet)); err != nil {
					slog.Error("Failed to delete existing virtual network in ETCD", "err", err, "networkId", api.VirtualNetworkID(existingNet))
					continue
				}
			}
		}

		// 既にETCDに登録されているか確認
		_, err = m.Db.GetVirtualNetworkById(api.VirtualNetworkID(*vnet))
		if err == nil {
			slog.Debug("Virtual network already exists in ETCD, skipping", "id", api.VirtualNetworkID(*vnet))
		} else if err == db.ErrNotFound {
			// このノードで発見したネットワークにノード名を付与する
			if m != nil && m.NodeName != "" {
				vnet.Metadata.NodeName = util.StringPtr(m.NodeName)
			}
			// データベースに登録
			if err := m.Db.PutVirtualNetworksETCD(*vnet); err != nil {
				slog.Error("Failed to put virtual network to ETCD", "err", err)
			}
		} else {
			slog.Error("Failed to check virtual network in ETCD", "err", err, "id", api.VirtualNetworkID(*vnet))
			continue
		}
		m.Db.UpdateVirtualNetworkStatus(api.VirtualNetworkID(*vnet), db.NETWORK_ACTIVE)
		if strings.TrimSpace(vnet.Metadata.Name) == "host-bridge" {
			if err := m.ensureHostBridgeIPNetwork(vnet); err != nil {
				slog.Error("Failed to sync host-bridge IP network from config", "err", err, "networkId", api.VirtualNetworkID(*vnet))
				continue
			}
		}
	}

	return nil
}

func networkDeletionRequested(network api.VirtualNetwork) bool {
	return network.Status != nil && network.Status.DeletionTimeStamp != nil
}

// virtual.Networkをapi.VirtualNetworkに変換する関数
func convertLibvirtNetworkToAPINetwork(libnet libvirt.Network) (*api.VirtualNetwork, error) {
	var net api.VirtualNetwork
	var meta api.Metadata
	var spec api.VirtualNetworkSpec
	net.Metadata = meta
	net.Spec = spec

	// name
	name, err := libnet.GetName()
	if err != nil {
		slog.Error("Failed to get virtual network name", "err", err)
		return nil, err
	}
	net.Metadata.Name = name

	// uuid
	uuid, err := libnet.GetUUIDString()
	if err != nil {
		slog.Error("Failed to get virtual network UUID", "err", err)
		return nil, err
	}
	net.Metadata.Uuid = util.StringPtr(uuid)
	api.SetVirtualNetworkID(&net, uuid[:5]) // IDはUUIDの先頭8文字を使用

	// Bridge
	bridge, err := libnet.GetBridgeName()
	if err != nil {
		slog.Error("Failed to get virtual network bridge name", "err", err)
		return nil, err
	}
	net.Spec.BridgeName = util.StringPtr(bridge)

	xml, err := libnet.GetXMLDesc(0)
	if err != nil {
		slog.Error("Failed to get virtual network XML description", "err", err)
		return nil, err
	}

	// libvirt XMLをパースして、APIのVirtualNetworkに変換る
	var libvirtxmlNet libvirtxml.Network
	err = libvirtxmlNet.Unmarshal(xml)
	if err != nil {
		return nil, err
	}

	if libvirtxmlNet.Forward != nil {
		net.Spec.ForwardMode = util.StringPtr(libvirtxmlNet.Forward.Mode)
		net.Spec.Nat = util.BoolPtr(libvirtxmlNet.Forward.NAT != nil)
	}
	if libvirtxmlNet.MAC != nil {
		net.Spec.MacAddress = util.StringPtr(libvirtxmlNet.MAC.Address)
	}
	if len(libvirtxmlNet.IPs) > 0 {
		net.Spec.IpAddress = util.StringPtr(libvirtxmlNet.IPs[0].Address)
		net.Spec.Netmask = util.StringPtr(libvirtxmlNet.IPs[0].Netmask)
		if libvirtxmlNet.IPs[0].DHCP != nil && len(libvirtxmlNet.IPs[0].DHCP.Ranges) > 0 {
			net.Spec.Dhcp = util.BoolPtr(true)
			net.Spec.DhcpStartAddress = util.StringPtr(libvirtxmlNet.IPs[0].DHCP.Ranges[0].Start)
			net.Spec.DhcpEndAddress = util.StringPtr(libvirtxmlNet.IPs[0].DHCP.Ranges[0].End)
		}
	}

	return &net, nil

}
