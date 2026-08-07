package marmotd

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func (s *Server) ApiCreateKubernetesEngine(ctx echo.Context) error {
	var rec api.KubernetesEngine
	if err := ctx.Bind(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	created, err := s.Ma.Db.CreateKubernetesEngine(rec)
	if err != nil {
		if errors.Is(err, db.ErrFound) {
			return ctx.JSON(http.StatusConflict, api.Error{Code: 1, Message: err.Error()})
		}
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, created)
}

func (s *Server) ApiGetKubernetesEngines(ctx echo.Context) error {
	items, err := s.Ma.Db.GetKubernetesEngines()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, items)
}

func (s *Server) ApiGetKubernetesEngineById(ctx echo.Context, id string) error {
	item, err := s.Ma.Db.GetKubernetesEngineById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, item)
}

func (s *Server) ApiDeleteKubernetesEngineById(ctx echo.Context, id string) error {
	if _, err := s.Ma.Db.GetKubernetesEngineById(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	// 実削除はコントローラーが DELETING 状態を検知し、猶予期間経過後に行う。
	if err := s.Ma.Db.SetDeleteTimestampKubernetesEngine(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to delete the kubernetes engine")})
}
