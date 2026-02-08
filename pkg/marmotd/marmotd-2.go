package marmotd

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

// ボリュームの生成 implements api.ServerInterface.
func (s *Server) CreateVolume(ctx echo.Context) error {
	slog.Debug("===", "CreateVolume() is called", "===")
	var volume api.Volume

	err := ctx.Bind(&volume)
	if err != nil {
		volumeString, err2 := json.MarshalIndent(volume, "", "  ")
		slog.Error("CreateVolume()", "err", err, "volume", string(volumeString), "err2", err2)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// キーだけでなく、スペック全体を返すようにする
	spec, err := s.Ma.CreateNewVolume(volume)
	if err != nil {
		slog.Error("CreateNewVolume()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("CreateVolume()", "volKey", *spec.Metadata.Key)

	return ctx.JSON(http.StatusCreated, spec)
}

// IDを指定してボリュームを削除 implements api.ServerInterface.
func (s *Server) DeleteVolumeById(ctx echo.Context, id string) error {
	slog.Debug("===", "DeleteVolumeById() is called", "===", "volumeId", id)
	if err := s.Ma.RemoveVolume(id); err != nil {
		slog.Error("RemoveVolume()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "Successfully deleted"})
}

// ボリュームのリストを取得 implements api.ServerInterface.
func (s *Server) ListVolumes(ctx echo.Context) error {
	slog.Debug("===", "ListVolumes() is called", "===")
	vols, err := s.Ma.GetDataVolumes()
	if err != nil {
		slog.Error("ListVolumes()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vols2, err := s.Ma.GetOsVolumes()
	if err != nil {
		slog.Error("ListVolumes()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vols = append(vols, vols2...)

	return ctx.JSON(http.StatusOK, vols)
}

// IDを指定してボリュームの詳細を取得 implements api.ServerInterface.
func (s *Server) ShowVolumeById(ctx echo.Context, volumeId string) error {
	slog.Debug("===", "ShowVolumeById() is called", "===", "volumeId", volumeId)
	vol, err := s.Ma.GetVolumeById(volumeId)
	if err != nil {
		slog.Error("ShowVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("ShowVolumeById()", "vol", vol)

	return ctx.JSON(http.StatusOK, vol)
}

// IDを指定してボリュームを更新 implements api.ServerInterface.
func (s *Server) UpdateVolumeById(ctx echo.Context, volumeId string) error {
	slog.Debug("===", "UpdateVolumeById() is called", "===", "volumeId", volumeId)
	var volume api.Volume
	if err := ctx.Bind(&volume); err != nil {
		slog.Error("UpdateVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	key := db.VolumePrefix + "/" + volumeId
	if _, err := s.Ma.UpdateVolumeById(volumeId, volume); err != nil {
		slog.Error("UpdateVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	volume.Metadata.Key = &key
	volume.Id = volumeId

	return ctx.JSON(http.StatusOK, volume)
}

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

	// ステータスを削除中に更新
	svc, err := s.Ma.GetServerById(id)
	if err != nil {
		slog.Error("GetServerById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	svc.Status.Status = util.IntPtrInt(db.SERVER_DELETING)
	svc.Status.DeletionTimeStamp = util.TimePtr(time.Now())
	if err := s.Ma.Db.UpdateServer(id, svc); err != nil {
		slog.Error("UpdateServer()", "err", err)
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
