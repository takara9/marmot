package marmotd

import (
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// サーバーのリストを取得、フィルターは、パラメータで指定するようにする
func (s *Server) ApiGetServers(ctx echo.Context) error {
	slog.Debug("===", "ApiGetServers() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("ApiGetServers()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	recs, err := s.Ma.GetServersManage()
	if err != nil {
		slog.Error("ApiGetServers()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, recs)
}

// サーバーの作成
func (s *Server) ApiCreateServer(ctx echo.Context) error {
	slog.Debug("===ApiCreateServer() is called===", "err", 0)

	//var serverSpec api.Server
	var virtualServer api.Server
	if err := ctx.Bind(&virtualServer); err != nil {
		slog.Error("ApiCreateServer()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("Recived post body", "serverSpec=", virtualServer, "cpu=", virtualServer.Spec.Cpu, "memory=", virtualServer.Spec.Memory, "os", virtualServer.Spec.OsVariant)

	// リクエストをetcdに登録し、正常応答を返す
	slog.Debug("仮想マシンの使用を付与してDBへ登録、一意のIDを取得")
	vm, err := s.Ma.Db.MakeServerEntry(virtualServer)
	if err != nil {
		slog.Error("ApiCreateServer()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("ApiCreateServer()", "Server Id", vm.Id)

	var resp api.Success
	resp.Id = vm.Id
	resp.Message = util.StringPtr("Server created successfully")
	return ctx.JSON(http.StatusOK, resp)
}

// サーバーの詳細を取得
func (s *Server) ApiGetServerById(ctx echo.Context, id string) error {
	slog.Debug("=== ApiGetServerById() is called ===", "id", id)
	server, err := s.Ma.GetServerManage(id)
	if err != nil {
		slog.Error("ApiGetServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, server)
}

// サーバーの削除
func (s *Server) ApiDeleteServerById(ctx echo.Context, id string) error {
	slog.Debug("===ApiDeleteServerById() is called ===", "id", id)

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
func (s *Server) ApiUpdateServerById(ctx echo.Context, id string) error {
	slog.Debug("===", "ApiUpdateServerById() is called", "===")
	var serverSpec api.Server
	if err := ctx.Bind(&serverSpec); err != nil {
		slog.Error("ApiUpdateServerById()", "err", err)
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

// サーバーからイメージを作成
func (s *Server) ApiMakeImageEntryFromRunningVMById(ctx echo.Context, serverId string) error {
	slog.Debug("=== ApiMakeImageEntryFromRunningVMById() is called ===", "id", serverId)

	// イメージのリクエストをDBへ登録、実際の処理はコントローラーに委ねる
	var image api.Image
	if err := ctx.Bind(&image); err != nil {
		slog.Error("ApiMakeImageEntryFromRunningVMById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// serverIdから、ブートボリュームIDを取得
	server, err := s.Ma.GetServerManage(serverId)
	if err != nil {
		slog.Error("ApiGetServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// 新しいイメージ名の有無チェック
	if image.Metadata.Name == nil && len(*image.Metadata.Name) > 0 {
		slog.Error("Image name is not set, it must set for new image", "err", err)
		err := fmt.Errorf("Must set image name")
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// イメージ作成の登録
	if _, err := s.Ma.Db.MakeImageEntryFromRunningVM(server.Id, *image.Metadata.Name); err != nil {
		slog.Error("Image name is not set, it must set for new image", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.Success{Id: serverId, Message: util.StringPtr("Image created successfully from server")})
}
