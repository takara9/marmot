package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

// サーバーの作成
func (m *MarmotEndpoint) CreateServer(spec api.Server) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateServer is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/server")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateServer", "reqURL", reqURL)

	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateServer", "body", string(byteJSON))

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// サーバーの削除
func (m *MarmotEndpoint) DeleteServerById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteServerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/server/"+id)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("DeleteServerById", "reqURL", reqURL)

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest2(req)
}

// サーバーの詳細を取得
func (m *MarmotEndpoint) GetServerById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetServerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/server/"+id)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("GetServerById", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// サーバーの更新
func (m *MarmotEndpoint) UpdateServerById(id string, spec api.Server) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateServerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/server/"+id)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("UpdateServerById", "reqURL", reqURL)

	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("UpdateServerById", "body", string(byteJSON))

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// サーバーの一覧を取得、フィルターは、パラメータで指定するようにする
func (m *MarmotEndpoint) GetServers() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetServers is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/server")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("GetServers", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

/*
// 仮想プライベートネットワークの作成
func (m *MarmotEndpoint) CreateVirtualNetwork(spec api.VirtualNetwork) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateVirtualNetwork is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateVirtualNetwork", "reqURL", reqURL)

	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("CreateVirtualNetwork", "body", string(byteJSON))

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// 仮想プライベートネットワークの削除
func (m *MarmotEndpoint) DeleteVirtualNetworkById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteVirtualNetworkById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network/"+id)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("DeleteVirtualNetworkById", "reqURL", reqURL)

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest2(req)
}

// 仮想プライベートネットワークの詳細を取得
func (m *MarmotEndpoint) GetVirtualNetworkById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetVirtualNetworkById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network/"+id)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("GetVirtualNetworkById", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// 仮想プライベートネットワークの更新
func (m *MarmotEndpoint) UpdateVirtualNetworkById(id string, spec api.VirtualNetwork) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateVirtualNetworkById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network/"+id)
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("UpdateVirtualNetworkById", "reqURL", reqURL)

	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}

	slog.Debug("UpdateVirtualNetworkById", "body", string(byteJSON))

	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// 仮想プライベートネットワークの一覧を取得、フィルターは、パラメータで指定するようにする
func (m *MarmotEndpoint) GetVirtualNetworks() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetVirtualNetworks is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network")
	if err != nil {
		return nil, nil, err
	}
	slog.Debug("GetVirtualNetworks", "reqURL", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}
*/
