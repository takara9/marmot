package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

// CreateGateway creates a gateway resource.
func (m *MarmotEndpoint) CreateGateway(spec api.Gateway) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateGateway is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/gateway")
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

// DeleteGatewayById requests deletion for a gateway resource.
func (m *MarmotEndpoint) DeleteGatewayById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteGatewayById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/gateway/"+id)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// GetGatewayById gets a gateway by ID.
func (m *MarmotEndpoint) GetGatewayById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetGatewayById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/gateway/"+id)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// UpdateGatewayById updates a gateway by ID.
func (m *MarmotEndpoint) UpdateGatewayById(id string, spec api.Gateway) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateGatewayById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/gateway/"+id)
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

// GetGateways gets all gateways.
func (m *MarmotEndpoint) GetGateways() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetGateways is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/gateway")
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}
