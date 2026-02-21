package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

// ボリュームの作成
func (m *MarmotEndpoint) CreateVolume(spec api.Volume) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/volume")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateVolume", "reqURL", reqURL, "spec", spec)

	byteJSON, _ := json.Marshal(spec)
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// ボリュームの削除
func (m *MarmotEndpoint) DeleteVolumeById(key string) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/volume", key)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateVolume", "reqURL", reqURL, "key", key)

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// ボリュームの一覧取得
func (m *MarmotEndpoint) ListVolumes() ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/volume")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("ListVolumes", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// ボリュームの詳細取得
func (m *MarmotEndpoint) ShowVolumeById(key string) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/volume", key)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("ShowVolumeById", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// ボリュームの更新
func (m *MarmotEndpoint) UpdateVolumeById(key string, spec api.Volume) ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/volume", key)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("UpdateVolumeById", "reqURL", reqURL)

	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("UpdateVolumeById", "body", string(byteJSON))

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}
