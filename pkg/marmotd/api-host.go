package marmotd

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func annotateIscsiServerStatuses(statuses []api.HostStatus) []api.HostStatus {
	annotated := make([]api.HostStatus, len(statuses))
	copy(annotated, statuses)

	for i := range annotated {
		if annotated[i].NodeName == nil {
			continue
		}
		nodeName := strings.TrimSpace(*annotated[i].NodeName)
		if nodeName == "" {
			continue
		}
		annotated[i].IscsiServer = util.BoolPtr(IsIscsiServer(nodeName, annotated))
	}

	return annotated
}

// ホストのステータスを取得するAPIハンドラー
func (s *Server) ApiGetMarmotStatus(ctx echo.Context) error {
	slog.Debug("===", "ApiGetMarmotStatus() is called", "===")

	statuses, err := s.Ma.Db.GetAllHostStatus()
	if err != nil {
		slog.Error("ApiGetMarmotStatus() GetAllHostStatus() failed", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	annotated := annotateIscsiServerStatuses(statuses)
	for _, status := range annotated {
		if status.NodeName != nil && *status.NodeName == s.Ma.NodeName {
			return ctx.JSON(http.StatusOK, status)
		}
	}

	// 既存動作互換: 自ノードが見つからない場合は直接取得して返す
	status, err := s.Ma.Db.GetHostStatus(s.Ma.NodeName)
	if err != nil {
		slog.Error("ApiGetMarmotStatus() GetHostStatus() failed", "err", err, "nodeName", s.Ma.NodeName)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, status)
}

// クラスタ全体のホストステータスを取得するAPIハンドラー
func (s *Server) ApiGetMarmotCluster(ctx echo.Context) error {
	slog.Debug("===", "ApiGetMarmotCluster() is called", "===")

	statuses, err := s.Ma.Db.GetAllHostStatus()
	if err != nil {
		slog.Error("ApiGetMarmotCluster() GetAllHostStatus() failed", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, annotateIscsiServerStatuses(statuses))
}
