package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
)

type marmotdServer struct{}

type MarmotEndpoint struct {
	Scheme   string // Scheme for the endpoint, e.g., "http" or "https".
	HostPort string
	BasePath string       // Base path for the API, e.g., "/api/v1".
	Client   *http.Client // Specialized client.
}

func startMockServer() {
	e := echo.New()
	server := Server{}
	go func() {
		api.RegisterHandlersWithBaseURL(e, server, "/api/v1")
		fmt.Println(e.Start("0.0.0.0:8080"), "Mock server is running")
	}()
}

func NewMarmotdEp(schame string, address string, basePath string, timeout int) (*MarmotEndpoint, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableCompression = true
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &MarmotEndpoint{
		Scheme:   "http",
		HostPort: address,
		BasePath: basePath,
		Client:   &http.Client{Transport: tr, Timeout: time.Duration(timeout) * time.Second},
	}, nil
}

func (m *MarmotEndpoint) httpRequest(req *http.Request) (int, []byte, *url.URL, error) {
	resp, err := m.Client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}

	byteJSON, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return 0, nil, nil, err
	}

	jobURL, err := resp.Location()
	if err != nil {
		if err.Error() == "http: no Location header in response" {
			return resp.StatusCode, byteJSON, nil, nil
		} else {
			return resp.StatusCode, byteJSON, nil, err
		}
	}

	return resp.StatusCode, byteJSON, jobURL, nil
}

func (m *MarmotEndpoint) setUrl(apiPath string) string {
	return fmt.Sprintf("%s://%s%s%s", m.Scheme, m.HostPort, m.BasePath, apiPath)
}
func (m *MarmotEndpoint) Ping() (int, []byte, *url.URL, error) {
	url := m.setUrl("/ping")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Accept", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) GetVersion() (int, []byte, *url.URL, error) {
	url := m.setUrl("/version")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Accept", "application/json")
	return m.httpRequest(req)
}
k
