package marmotd

import (
	_ "embed"
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

// クライアントから仮想ネットワークの情報を受け取って、仮想ネットワークを作成する
// ここでは、etcdに書き込むだけで、実際に仮想ネットワークを作成するのはコントローラー側で行う
func (s *Server) CreateNetwork(ctx echo.Context) error {
	// クライアントから仮想ネットワークの情報を受け取る
	var spec api.VirtualNetwork

	if err := ctx.Bind(&spec); err != nil {
		slog.Error("failed to bind request body", "err", err)
		return echo.NewHTTPError(400, "invalid request body")
	}

	// 仮想ネットワークを作成する
	network, err := s.Ma.Db.CreateVirtualNetwork(spec)
	if err != nil {
		slog.Error("failed to create virtual network", "err", err)
		return echo.NewHTTPError(500, "failed to create virtual network")
	}

	return ctx.JSON(201, network)
}

// 仮想ネットワークをIDで削除する
// 実際に削除するのはコントローラー側で、ここではDBに削除タイムスタンプを設定するだけ
func (s *Server) DeleteNetworkById(ctx echo.Context, id string) error {
	if err := s.Ma.Db.DeleteVirtualNetworkById(id); err != nil {
		slog.Error("failed to delete virtual network", "err", err, "networkId", id)
		return echo.NewHTTPError(500, "failed to delete virtual network")
	}
	return ctx.NoContent(204)
}

// クライアントから仮想ネットワークの情報を受け取って、仮想ネットワークをIDで更新する
// ここでは、etcdに書き込むだけで、実際に仮想ネットワークを更新するのはコントローラー側で行う
func (s *Server) UpdateNetworkById(ctx echo.Context, id string) error {
	var spec api.VirtualNetwork
	if err := ctx.Bind(&spec); err != nil {
		slog.Error("failed to bind request body", "err", err)
		return echo.NewHTTPError(400, "invalid request body")
	}

	if err := s.Ma.Db.UpdateVirtualNetworkById(id, spec); err != nil {
		slog.Error("failed to update virtual network", "err", err, "networkId", id)
		return echo.NewHTTPError(500, "failed to update virtual network")
	}

	return ctx.NoContent(204)
}

// 参照のみ
// 仮想ネットワークのリストを取得して返す
func (s *Server) GetNetworks(ctx echo.Context) error {
	slog.Debug("===", "GetNetworks is called", "===")
	networks, err := s.Ma.Db.GetVirtualNetworks()
	if err != nil {
		slog.Error("failed to get virtual networks", "err", err)
		return echo.NewHTTPError(500, "failed to get virtual networks")
	}
	return ctx.JSON(200, networks)
}

// 参照のみ
// 仮想ネットワークをIDで取得する
func (s *Server) GetNetworkById(ctx echo.Context, id string) error {
	network, err := s.Ma.Db.GetVirtualNetworkById(id)
	if err != nil {
		slog.Error("failed to get virtual network", "err", err, "networkId", id)
		return echo.NewHTTPError(500, "failed to get virtual network")
	}
	return ctx.JSON(200, network)
}
