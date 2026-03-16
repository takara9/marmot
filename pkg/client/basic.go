package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/takara9/marmot/api"
)

func (m *MarmotEndpoint) Ping() (int, []byte, *url.URL, error) {
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/ping")
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) GetVersion() (*api.Version, error) {
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/version")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	status, body, _, err := m.httpRequest(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("http status code = %d", status)
	}
	var ret api.Version
	err = json.Unmarshal(body, &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}
