package marmotd

import (
	_ "embed"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// サーバーのリストを取得、フィルターは、パラメータで指定するようにする
func (s *Server) GetServers(ctx echo.Context) error {
	slog.Debug("===", "GetServers() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("GetServers()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	recs, err := s.Ma.GetServers()
	if err != nil {
		slog.Error("GetServers()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, recs)
}

// サーバーの作成
func (s *Server) CreateServer(ctx echo.Context) error {
	slog.Debug("===CreateServer() is called===", "err", 0)

	//var serverSpec api.Server
	var virtualServer api.Server
	if err := ctx.Bind(&virtualServer); err != nil {
		slog.Error("CreateServer()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("Recived post body", "serverSpec=", virtualServer, "cpu=", virtualServer.Spec.Cpu, "memory=", virtualServer.Spec.Memory, "os", virtualServer.Spec.OsVariant)

	// リクエストをetcdに登録し、正常応答を返す
	slog.Debug("仮想マシンの使用を付与してDBへ登録、一意のIDを取得")
	vm, err := s.Ma.Db.CreateServer(virtualServer)
	if err != nil {
		slog.Error("CreateServer()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("CreateServer()", "Server Id", vm.Id)

	var resp api.Success
	resp.Id = vm.Id
	resp.Message = util.StringPtr("Server created successfully")
	return ctx.JSON(http.StatusOK, resp)
}

// サーバーの詳細を取得
func (s *Server) GetServerById(ctx echo.Context, id string) error {
	slog.Debug("=== GetServerById() is called ===", "id", id)
	server, err := s.Ma.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, server)
}

// サーバーの削除
func (s *Server) DeleteServerById(ctx echo.Context, id string) error {
	slog.Debug("===DeleteServerById() is called ===", "id", id)

	if err := s.Ma.Db.SetDeleteTimestamp(id); err != nil {
		slog.Error("SetDeleteTimestamp()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Accepted the request to delete the server")
	return ctx.JSON(http.StatusOK, resp)
}

// サーバーの更新
func (s *Server) UpdateServerById(ctx echo.Context, id string) error {
	slog.Debug("===", "UpdateServerById() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("DeleteServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	if err := s.Ma.UpdateServerById(id, serverSpec); err != nil {
		slog.Error("UpdateServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Server updated successfully")
	return ctx.JSON(http.StatusOK, resp)
}
