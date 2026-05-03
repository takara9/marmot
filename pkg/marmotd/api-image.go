package marmotd

// イメージのAPIハンドラー群

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func (s *Server) ApiCreateImage(ctx echo.Context) error {
	slog.Debug("===ApiCreateImage() is called===", "err", 0)

	var imageSpec api.Image
	if err := ctx.Bind(&imageSpec); err != nil {
		slog.Error("ApiCreateImage()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("Recived post body", "imageSpec=", imageSpec, "sourceUrl=", imageSpec.Spec.SourceUrl)

	// リクエストをetcdに登録し、正常応答を返す
	slog.Debug("イメージの使用を付与してDBへ登録、一意のIDを取得")
	// URLの設定チェック
	if imageSpec.Spec == nil || imageSpec.Spec.SourceUrl == nil {
		slog.Error("ApiCreateImage()", "err", "SourceUrl is required")
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "SourceUrl is required"})
	}
	// 名前の設定チェック
	if imageSpec.Metadata == nil || imageSpec.Metadata.Name == nil {
		slog.Error("ApiCreateImage()", "err", "Name is required")
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "Name is required"})
	}
	assignNodeNameIfUnset(&imageSpec.Metadata, s.Ma.NodeName)
	assignedNodeName := ""
	if imageSpec.Metadata != nil && imageSpec.Metadata.NodeName != nil {
		assignedNodeName = *imageSpec.Metadata.NodeName
	}

	id, err := s.Ma.Db.MakeImageEntryFromURLWithNode(*imageSpec.Metadata.Name, *imageSpec.Spec.SourceUrl, assignedNodeName)
	if err != nil {
		slog.Error("ApiCreateImage()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("ApiCreateImage()", "Image Id", id)

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Image created successfully")
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) ApiGetImages(ctx echo.Context) error {
	slog.Debug("===", "ApiGetImages() is called", "===")
	var imageSpec api.Image
	if err := ctx.Bind(&imageSpec); err != nil {
		slog.Error("ApiGetImages()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// 直接DB関数を呼ばないで、マネージャーの関数を呼ぶようにする
	// そうすることで、マネージャーの関数もテストできるようになる
	recs, err := s.Ma.GetImagesManage()
	if err != nil {
		slog.Error("ApiGetImages()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, recs)
}

func (s *Server) ApiGetImageById(ctx echo.Context, id string) error {
	slog.Debug("=== ApiGetImageById() is called ===", "id", id)

	// 直接DB関数を呼ばないで、マネージャーの関数を呼ぶようにする
	// そうすることで、マネージャーの関数もテストできるようになる
	image, err := s.Ma.GetImageManage(id)
	if err != nil {
		slog.Error("ApiGetImageById()", "err", err)
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, image)
}

func (s *Server) ApiDeleteImageById(ctx echo.Context, id string) error {
	slog.Debug("===ApiDeleteImageById() is called ===", "id", id)

	// 指定 ID のイメージ名を取得
	target, err := s.Ma.Db.GetImage(id)
	if err != nil {
		slog.Error("ApiDeleteImageById() GetImage failed", "id", id, "err", err)
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	if target.Metadata == nil || target.Metadata.Name == nil {
		slog.Error("ApiDeleteImageById() image name is empty", "id", id)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: "image name is empty"})
	}
	targetName := *target.Metadata.Name

	// 同じ IMAGE-NAME を持つ全イメージに削除タイムスタンプをセット
	allImages, err := s.Ma.Db.GetImages()
	if err != nil {
		slog.Error("ApiDeleteImageById() GetImages failed", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	for _, img := range allImages {
		if img.Metadata == nil || img.Metadata.Name == nil || *img.Metadata.Name != targetName {
			continue
		}
		if img.Status != nil && img.Status.DeletionTimeStamp != nil {
			// 既に削除予定済みのものはスキップ
			continue
		}
		if setErr := s.Ma.Db.SetDeleteTimestampImage(img.Id); setErr != nil {
			slog.Error("ApiDeleteImageById() SetDeleteTimestampImage failed", "imageId", img.Id, "err", setErr)
		} else {
			slog.Debug("ApiDeleteImageById() 削除タイムスタンプをセット", "imageId", img.Id, "imageName", targetName)
		}
	}

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Image deleted successfully")
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) ApiUpdateImageById(ctx echo.Context, id string) error {
	slog.Debug("===", "ApiUpdateImageById() is called", "===")
	var imageSpec api.Image
	if err := ctx.Bind(&imageSpec); err != nil {
		slog.Error("ApiUpdateImageById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	err := s.Ma.UpdateImageManage(id, imageSpec)
	if err != nil {
		slog.Error("ApiUpdateImageById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Image updated successfully")
	return ctx.JSON(http.StatusOK, resp)
}
