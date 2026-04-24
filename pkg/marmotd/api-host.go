package marmotd

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

// ホストのステータスを取得するAPIハンドラー
func (s *Server) ApiGetMarmotStatus(ctx echo.Context) error {
	slog.Debug("===", "ApiGetMarmotStatus() is called", "===")

	status, err := s.Ma.Db.GetHostStatus(s.Ma.NodeName)
	if err != nil {
		slog.Error("ApiGetMarmotStatus() GetHostStatus() failed", "err", err, "nodeName", s.Ma.NodeName)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, status)
}
