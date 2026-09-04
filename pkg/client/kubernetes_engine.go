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

func (m *MarmotEndpoint) UpdateKubernetesEngineById(id string, spec api.KubernetesEngine) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateKubernetesEngineById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/kubernetes-engine/"+id)
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

// GetKubernetesEngineKubeconfigById downloads the admin kubeconfig for a
// KubernetesEngine cluster as text/plain.
func (m *MarmotEndpoint) GetKubernetesEngineKubeconfigById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetKubernetesEngineKubeconfigById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/kubernetes-engine/"+id+"/kubeconfig")
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/plain")
	if token := strings.TrimSpace(m.AccessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	status, body, jobURL, err := m.httpRequest(req)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, fmt.Errorf("http status code = %d: %s", status, strings.TrimSpace(string(body)))
	}
	return body, jobURL, nil
}
