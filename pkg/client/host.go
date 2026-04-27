package client

import (
	"net/http"
	"net/url"
)

// ホストのステータスを取得
func (m *MarmotEndpoint) GetMarmotStatus() ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/marmot/status")
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}

// クラスタ全体のホストステータスを取得
func (m *MarmotEndpoint) GetMarmotCluster() ([]byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/marmot/cluster")
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return m.httpRequest2(req)
}
