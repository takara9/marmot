package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/takara9/marmot/api"
)

func (m *MarmotEndpoint) CreateVpnGateway(spec api.VpnGateway) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateVpnGateway is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/vpn-gateway")
	if err != nil {
		return nil, nil, err
	}
	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetVpnGateways() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetVpnGateways is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/vpn-gateway")
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetVpnGatewayById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetVpnGatewayById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/vpn-gateway/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) UpdateVpnGatewayById(id string, spec api.VpnGateway) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateVpnGatewayById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/vpn-gateway/"+id)
	if err != nil {
		return nil, nil, err
	}
	byteJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) DeleteVpnGatewayById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteVpnGatewayById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/vpn-gateway/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetVpnGatewayCertById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetVpnGatewayCertById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/vpn-gateway/"+id+"/cert")
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/plain")
	status, body, jobURL, err := m.httpRequest(req)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, fmt.Errorf("http status code = %d: %s", status, strings.TrimSpace(string(body)))
	}
	return body, jobURL, nil
}
