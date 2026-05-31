package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

func (m *MarmotEndpoint) CreateLoadBalancer(spec api.LoadBalancer) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateLoadBalancer is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/load-balancer")
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

func (m *MarmotEndpoint) GetLoadBalancers() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetLoadBalancers is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/load-balancer")
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetLoadBalancerById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetLoadBalancerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/load-balancer/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) UpdateLoadBalancerById(id string, spec api.LoadBalancer) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateLoadBalancerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/load-balancer/"+id)
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

func (m *MarmotEndpoint) DeleteLoadBalancerById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteLoadBalancerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/load-balancer/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}