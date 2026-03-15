package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

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
