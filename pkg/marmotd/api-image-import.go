package marmotd

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// ApiImportImageArchive imports a tgz archive that contains one qcow2 file.
func (s *Server) ApiImportImageArchive(ctx echo.Context) error {
	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "file form field is required"})
	}

	src, err := fileHeader.Open()
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	defer src.Close()

	baseName := strings.TrimSuffix(filepath.Base(fileHeader.Filename), filepath.Ext(fileHeader.Filename))
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	baseName = strings.TrimPrefix(baseName, "marmot-machine-image-")
	if strings.TrimSpace(baseName) == "" {
		baseName = "imported-image"
	}

	image, err := s.Ma.ImportImageArchiveWithNode(src, baseName, s.Ma.NodeName)
	if err != nil {
		slog.Error("ApiImportImageArchive() failed", "err", err)
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}

	var resp api.Success
	resp.Id = image.Metadata.Id
	resp.Message = util.StringPtr("Image imported successfully")
	return ctx.JSON(http.StatusOK, resp)
}

func extractSingleQcow2FromTGZ(src io.Reader, destDir string) (string, error) {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return "", fmt.Errorf("failed to read gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var found string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar stream: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		base := filepath.Base(hdr.Name)
		if !strings.HasSuffix(strings.ToLower(base), ".qcow2") {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("archive contains multiple qcow2 files")
		}

		dstPath := filepath.Join(destDir, base)
		f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		found = dstPath
	}

	if found == "" {
		return "", fmt.Errorf("archive does not contain qcow2 file")
	}
	return found, nil
}
