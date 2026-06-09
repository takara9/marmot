package client

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/takara9/marmot/api"
)

// イメージの作成
func (m *MarmotEndpoint) CreateImage(spec api.Image) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateImage", "reqURL", reqURL, "spec", spec)

	byteJSON, _ := json.Marshal(spec)
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// イメージの削除
func (m *MarmotEndpoint) DeleteImageById(key string) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image", key)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("DeleteImageById", "reqURL", reqURL, "key", key)

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// イメージの一覧取得
func (m *MarmotEndpoint) GetImages() ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("GetImages", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// イメージの詳細取得
func (m *MarmotEndpoint) ShowImageById(key string) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image", key)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("ShowImageById", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// イメージの更新
func (m *MarmotEndpoint) UpdateImageById(key string, spec api.Image) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image", key)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("UpdateImageById", "reqURL", reqURL)

	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("UpdateImageById", "body", string(byteJSON))

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// イメージのQCOW2をダウンロードする
func (m *MarmotEndpoint) DownloadImageQcow2ById(key string) ([]byte, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image", key, "qcow2")
	if err != nil {
		return nil, err
	}

	slog.Debug("DownloadImageQcow2ById", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	body, _, err := m.httpRequest2(req)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// tgzファイルをアップロードしてイメージをインポートする
func (m *MarmotEndpoint) ImportImageArchive(tgzPath string) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/image/import")
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(tgzPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(tgzPath))
	if err != nil {
		return nil, nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("POST", reqURL, &body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return m.httpRequest2(req)
}
