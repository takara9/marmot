package marmotd

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	"github.com/takara9/marmot/pkg/virt"
	"libvirt.org/go/libvirtxml"
)

// 起動時に既存ネットワークを取得して、データベースへの登録
func (m *Marmot) GetVirtualNetworksAndPutDB() ([]api.VirtualNetwork, error) {
	slog.Debug("GetVirtualNetworks called")
	var apiNetworks []api.VirtualNetwork

	networks, err := m.Virt.GetVirtualNetworks()
	if err != nil {
		slog.Error("Failed to get virtual networks from libvirt", "err", err)
		return nil, err
	}

	for _, n := range *networks {
		var net api.VirtualNetwork
		var meta api.Metadata
		var spec api.VirtualNetworkSpec
		net.Metadata = &meta
		net.Spec = &spec

		// name
		name, err := n.GetName()
		if err != nil {
			slog.Error("Failed to get virtual network name", "err", err)
			continue
		}
		net.Metadata.Name = util.StringPtr(name)

		// uuid
		uuid, err := n.GetUUIDString()
		if err != nil {
			slog.Error("Failed to get virtual network UUID", "err", err)
			continue
		}
		net.Metadata.Uuid = util.StringPtr(uuid)
		net.Id = uuid[:5] // IDはUUIDの先頭8文字を使用

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
		fmt.Println(string(xml))

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

		// 同じ名前のネットワークが既にETCDに登録されているか確認
		existingNet, err := m.Db.GetVirtualNetworkByName(name)
		if err == nil {
			if existingNet.Metadata.Uuid != net.Metadata.Uuid {
				err := m.Db.DeleteVirtualNetworkById(existingNet.Id)
				if err != nil {
					slog.Error("Failed to delete existing virtual network in ETCD", "err", err, "networkId", existingNet.Id)
					continue
				}
			}
		}

		// 既にETCDに登録されているか確認
		_, err = m.Db.GetVirtualNetworkById(net.Id)
		if err == nil {
			slog.Info("Virtual network already exists in ETCD, skipping", "id", net.Id)
			continue
		} else if err == db.ErrNotFound {
			// データベースに登録
			if err := m.Db.PutVirtualNetworksETCD(net); err != nil {
				slog.Error("Failed to put virtual network to ETCD", "err", err)
			}
		}

		// 仮想ネットワークの状態をACTIVEに更新
		//net.Status = &api.Status{
		//	Status: util.IntPtrInt(db.NETWORK_ACTIVE),
		//}
		m.Db.UpdateVirtualNetworkStatus(net.Id, db.NETWORK_ACTIVE)

		// 戻り値に追加
		apiNetworks = append(apiNetworks, net)
	}
	return apiNetworks, nil
}

// 仮想ネットワークの作成
// この関数は、PENDING状態のオブジェクトを受け取ることを想定している
func (m *Marmot) DeployVirtualNetwork(vnet api.VirtualNetwork) error {
	slog.Debug("DeployVirtualNetwork called", "vnet", vnet.Id)

	// 仮想ネットワークの状態をPROVISIONINGに更新
	m.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_PROVISIONING)

	// 仮想ネットワークのXMLを作成
	xml, err := virt.CreateVirtualNetworkXML(vnet)
	if err != nil {
		slog.Error("Failed to create virtual network XML", "err", err)
		return err
	}
	slog.Debug("Virtual network XML created", "id", vnet.Id)

	// IPネットワークの指定があるかの判定
	if vnet.Spec.IPNetworkAddress == nil {
		// 任意のIPネットワーク作成
		id, err := m.Db.CreateAnyIpNetwork(vnet.Id)
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
		id, err := m.Db.CreateIpNetwork(vnet.Id, &ipNetworkSpec)
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
	if err := m.Db.UpdateVirtualNetworkById(vnet.Id, vnet); err != nil {
		slog.Error("Failed to update virtual network", "err", err)
		return err
	}

	// 仮想ネットワークの作成の成功したか確認は？
	// コントローラーにまかせるか？

	// 仮想ネットワークの状態をACTIVEに更新
	m.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ACTIVE)
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

	fmt.Println("==== Deleting virtual network:", *vnet.Metadata.Name, "====")
	jsonBytes, err := json.MarshalIndent(vnet, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal virtual network for deletion", "err", err)
	} else {
		fmt.Println(string(jsonBytes))
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
	if err := m.Virt.DeleteVirtualNetwork(*vnet.Metadata.Name); err != nil {
		m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ERROR)
		slog.Error("Failed to delete virtual network", "err", err)
		//return err
	}

	// 実態が消えたら、データベースからも削除する
	// 仮想ネットワークのDB　削除処理を実装
	// --- IPが関連付いている場合は先にIPを削除する必要がある
	if vnet.Spec.IpNetworkId != nil {
		// 紐付いたIPネットワークを削除
		if err := m.Db.DeleteVirtualNetworkById(vnet.Id); err != nil {
			m.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_ERROR)
			slog.Error("Failed to delete virtual network", "err", err)
			return err
		}
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
