package marmotd

import (
	"log/slog"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

// 仮想ネットワークの作成
func (s *Server) CreateVirtualNetwork(vnet api.VirtualNetwork) {
	slog.Debug("CreateVirtualNetwork called", "vnet", vnet.Id)

	// 仮想ネットワークの状態をPROVISIONINGに更新
	s.Ma.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_PROVISIONING)

	//if err := virt.CreateVirtualNetworkXML(vnet); err != nil {
	//	slog.Error("Failed to create virtual network XML", "err", err)
	//	s.Ma.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ERROR)
	//	return
	//}

	// 仮想ネットワークの状態をACTIVEに更新
	s.Ma.Db.UpdateVirtualNetworkStatus(vnet.Id, db.NETWORK_ACTIVE)
}

// 仮想ネットワークの削除
func (s *Server) DeleteVirtualNetwork(networkId string) {
	slog.Debug("DeleteVirtualNetwork called", "networkId", networkId)

	// 使用中の仮想ネットワークは削除できないようにする

	// 仮想ネットワークの状態をDELETINGに更新
	s.Ma.Db.UpdateVirtualNetworkStatus(networkId, db.NETWORK_DELETING)

	// 仮想ネットワークの削除処理を実装
	// ここでは仮想ネットワークの削除処理は省略

	// 仮想ネットワークの状態を削除済みに更新
	if err := s.Ma.Db.DeleteVirtualNetworkById(networkId); err != nil {
		slog.Error("Failed to delete virtual network", "err", err)
	}
}
