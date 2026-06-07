package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

func (m *MarmotEndpoint) CreateNetworkLoadBalancer(spec api.NetworkLoadBalancer) ([]byte, *url.URL, error) {
	slog.Debug("===", "CreateNetworkLoadBalancer is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network-load-balancer")
	if err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetNetworkLoadBalancers() ([]byte, *url.URL, error) {
	slog.Debug("===", "GetNetworkLoadBalancers is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network-load-balancer")
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) GetNetworkLoadBalancerById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "GetNetworkLoadBalancerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network-load-balancer/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) UpdateNetworkLoadBalancerById(id string, spec api.NetworkLoadBalancer) ([]byte, *url.URL, error) {
	slog.Debug("===", "UpdateNetworkLoadBalancerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network-load-balancer/"+id)
	if err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("PUT", reqURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

func (m *MarmotEndpoint) DeleteNetworkLoadBalancerById(id string) ([]byte, *url.URL, error) {
	slog.Debug("===", "DeleteNetworkLoadBalancerById is called", "===")
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/network-load-balancer/"+id)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}
