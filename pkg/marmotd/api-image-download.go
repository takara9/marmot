package marmotd

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

// ApiDownloadImageQcow2ById returns the qcow2 file for the specified image id.
func (s *Server) ApiDownloadImageQcow2ById(ctx echo.Context) error {
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "image id is required"})
	}

	image, err := s.Ma.GetImageManage(id)
	if err != nil {
		slog.Error("ApiDownloadImageQcow2ById() failed to get image", "id", id, "err", err)
		return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: err.Error()})
	}
	if image.Spec == nil || image.Spec.Qcow2Path == nil || strings.TrimSpace(*image.Spec.Qcow2Path) == "" {
		return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "qcow2 path is not set"})
	}

	path := strings.TrimSpace(*image.Spec.Qcow2Path)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "qcow2 file does not exist"})
		}
		slog.Error("ApiDownloadImageQcow2ById() failed to stat qcow2", "id", id, "path", path, "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.File(path)
}
