package marmotd

// イメージのAPIハンドラー群

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func (s *Server) CreateImage(ctx echo.Context) error {
	slog.Debug("===CreateImage() is called===", "err", 0)

	var imageSpec api.Image
	if err := ctx.Bind(&imageSpec); err != nil {
		slog.Error("CreateImage()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("Recived post body", "imageSpec=", imageSpec, "sourceUrl=", imageSpec.Spec.SourceUrl)

	// リクエストをetcdに登録し、正常応答を返す
	slog.Debug("イメージの使用を付与してDBへ登録、一意のIDを取得")
	// URLの設定チェック
	if imageSpec.Spec == nil || imageSpec.Spec.SourceUrl == nil {
		slog.Error("CreateImage()", "err", "SourceUrl is required")
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "SourceUrl is required"})
	}
	// 名前の設定チェック
	if imageSpec.Metadata == nil || imageSpec.Metadata.Name == nil {
		slog.Error("CreateImage()", "err", "Name is required")
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "Name is required"})
	}

	id, err := s.Ma.Db.CreateImageFromURL(*imageSpec.Metadata.Name, *imageSpec.Spec.SourceUrl)
	if err != nil {
		slog.Error("CreateImage()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	slog.Debug("CreateImage()", "Image Id", id)

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Image created successfully")
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) GetImages(ctx echo.Context) error {
	slog.Debug("===", "GetImages() is called", "===")
	var imageSpec api.Image
	if err := ctx.Bind(&imageSpec); err != nil {
		slog.Error("GetImages()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	// 直接DB関数を呼ばないで、マネージャーの関数を呼ぶようにする
	// そうすることで、マネージャーの関数もテストできるようになる
	recs, err := s.Ma.GetImages()
	if err != nil {
		slog.Error("GetImages()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.JSON(http.StatusOK, recs)
}

func (s *Server) GetImageById(ctx echo.Context, id string) error {
	slog.Debug("=== GetImageById() is called ===", "id", id)

	// 直接DB関数を呼ばないで、マネージャーの関数を呼ぶようにする
	// そうすることで、マネージャーの関数もテストできるようになる
	image, err := s.Ma.GetImage(id)
	if err != nil {
		slog.Error("GetImageById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, image)
}

func (s *Server) DeleteImageById(ctx echo.Context, id string) error {
	slog.Debug("===DeleteImageById() is called ===", "id", id)

	// 直接DB関数を呼ばないで、マネージャーの関数を呼ぶようにする
	// そうすることで、マネージャーの関数もテストできるようになる
	// DelitionTimeをセットするだけで、DBからは削除しないようにする

	//err := s.Ma.DeleteImage(id)
	//if err != nil {
	//	slog.Error("DeleteImageById()", "err", err)
	//	return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	//}
	err := s.Ma.Db.SetDeleteTimestampImage(id)
	if err != nil {
		slog.Error("DeleteImageById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Image deleted successfully")
	return ctx.JSON(http.StatusOK, resp)
}

func (s *Server) UpdateImageById(ctx echo.Context, id string) error {
	slog.Debug("===", "UpdateImageById() is called", "===")
	var imageSpec api.Image
	if err := ctx.Bind(&imageSpec); err != nil {
		slog.Error("UpdateImageById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	err := s.Ma.UpdateImage(id, imageSpec)
	if err != nil {
		slog.Error("UpdateImageById()", "err", err)
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	var resp api.Success
	resp.Id = id
	resp.Message = util.StringPtr("Image updated successfully")
	return ctx.JSON(http.StatusOK, resp)
}
