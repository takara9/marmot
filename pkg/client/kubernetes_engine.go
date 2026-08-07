package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

func (m *MarmotEndpoint) CreateKubernetesEngine(spec api.KubernetesEngine) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateKubernetesEngine is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/kubernetes-engine")
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

func (m *MarmotEndpoint) GetKubernetesEngines() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetKubernetesEngines is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/kubernetes-engine")
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetKubernetesEngineById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetKubernetesEngineById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/kubernetes-engine/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) DeleteKubernetesEngineById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteKubernetesEngineById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/kubernetes-engine/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}
