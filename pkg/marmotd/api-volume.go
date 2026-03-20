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
func (s *Server) ApiCreateVolume(ctx echo.Context) error {
	slog.Debug("===", "ApiCreateVolume() is called", "===")
	var volume api.Volume

	err := ctx.Bind(&volume)
	if err != nil {
		volumeString, err2 := json.MarshalIndent(volume, "", "  ")
		slog.Error("ApiCreateVolume()", "err", err, "volume", string(volumeString), "err2", err2)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// etcdへの登録と状態の変更だけにして、実際のボリュームの作成はコントローラーが実施する
	requestedVolume, err := s.Ma.Db.CreateVolumeOnDB2(volume)
	if err != nil {
		slog.Error("ApiCreateVolume()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("ApiCreateVolume()", "volKey", *requestedVolume.Metadata.Key)

	return ctx.JSON(http.StatusCreated, requestedVolume)

}

// IDを指定してボリュームを削除 implements api.ServerInterface.
func (s *Server) ApiDeleteVolumeById(ctx echo.Context, id string) error {
	slog.Debug("===", "ApiDeleteVolumeById() is called", "===", "volumeId", id)

	// レコードは状態だけを変更して、実際の削除はコントローラーが実施する
	v := api.Volume{
		Status: &api.Status{
			Status:              util.IntPtrInt(db.VOLUME_DELETING),
			DeletionTimeStamp:   util.TimePtr(time.Now()),
			LastUpdateTimeStamp: util.TimePtr(time.Now()),
		},
	}
	if err := s.Ma.Db.UpdateVolume(id, v); err != nil {
		slog.Error("ApiDeleteVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, api.ReplyMessage{Message: "Successfully deleted"})
}

// ボリュームのリストを取得 implements api.ServerInterface.
func (s *Server) ApiListVolumes(ctx echo.Context) error {
	slog.Debug("===", "ApiListVolumes() is called", "===")
	vols, err := s.Ma.GetDataVolumes()
	if err != nil {
		slog.Error("ApiListVolumes()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vols2, err := s.Ma.GetOsVolumes()
	if err != nil {
		slog.Error("ApiListVolumes()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	vols = append(vols, vols2...)

	return ctx.JSON(http.StatusOK, vols)
}

// IDを指定してボリュームの詳細を取得 implements api.ServerInterface.
func (s *Server) ApiShowVolumeById(ctx echo.Context, volumeId string) error {
	slog.Debug("===", "ApiShowVolumeById() is called", "===", "volumeId", volumeId)
	vol, err := s.Ma.GetVolumeById(volumeId)
	if err != nil {
		slog.Error("ApiShowVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("ApiShowVolumeById()", "vol", vol)

	return ctx.JSON(http.StatusOK, vol)
}

// IDを指定してボリュームを更新 implements api.ServerInterface.
func (s *Server) ApiUpdateVolumeById(ctx echo.Context, volumeId string) error {
	slog.Debug("===", "ApiUpdateVolumeById() is called", "===", "volumeId", volumeId)
	var volume api.Volume
	if err := ctx.Bind(&volume); err != nil {
		slog.Error("ApiUpdateVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	key := db.VolumePrefix + "/" + volumeId
	if _, err := s.Ma.UpdateVolumeById(volumeId, volume); err != nil {
		slog.Error("ApiUpdateVolumeById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	volume.Metadata.Key = &key
	volume.Id = volumeId

	return ctx.JSON(http.StatusOK, volume)
}
