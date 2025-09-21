package marmot

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
)

type MarmotEndpoint struct {
	Scheme   string // Scheme for the endpoint, e.g., "http" or "https".
	HostPort string
	BasePath string       // Base path for the API, e.g., "/api/v1".
	Client   *http.Client // Specialized client.
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

func (m *MarmotEndpoint) GetVersion() (int, []byte, *url.URL, error) {
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/version")
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) ListHypervisors(params map[string]string) (int, []byte, *url.URL, error) {
	fmt.Println("Client: ListHypervisors")
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/hypervisors")
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")

	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return m.httpRequest(req)
}

func (m *MarmotEndpoint) GetHypervisor(nodeName string) (int, []byte, *url.URL, error) {
	fmt.Println("Client: GetHypervisor")
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/hypervisor/"+nodeName)
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")

	return m.httpRequest(req)
}

func (m *MarmotEndpoint) ListVirtualMachines(params map[string]string) (int, []byte, *url.URL, error) {
	fmt.Println("Client: ListVirtualMachines")
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/virtualMachines")
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")

	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return m.httpRequest(req)
}


func (m *MarmotEndpoint) CreateCluster(params config.MarmotConfig) (int, []byte, *url.URL, error) {
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return 0, nil, nil, err
	}

	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/createCluster")
	if err != nil {
		return 0, nil, nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return 0, nil, nil, err
	}

	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) DestroyCluster(params config.MarmotConfig) (int, []byte, *url.URL, error) {
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return 0, nil, nil, err
	}

	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/destroyCluster")
	if err != nil {
		return 0, nil, nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return 0, nil, nil, err
	}

	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) StopCluster(params config.MarmotConfig) (int, []byte, *url.URL, error) {
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return 0, nil, nil, err
	}

	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/stopCluster")
	if err != nil {
		return 0, nil, nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return 0, nil, nil, err
	}

	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) StartCluster(params config.MarmotConfig) (int, []byte, *url.URL, error) {
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return 0, nil, nil, err
	}

	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/startCluster")
	if err != nil {
		return 0, nil, nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return 0, nil, nil, err
	}

	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) CreateVirtualMachine(node string, spec api.VmSpec) (int, []byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/createVm")
	if err != nil {
		return 0, nil, nil, err
	}
	byteJSON, _ := json.Marshal(spec)

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("", "err", err)
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) DestroyVirtualMachine(node string, spec api.VmSpec) (int, []byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/destroyVm")
	if err != nil {
		return 0, nil, nil, err
	}
	byteJSON, _ := json.Marshal(spec)

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("", "err", err)
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) StopVirtualMachine(node string, spec api.VmSpec) (int, []byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/stopVm")
	if err != nil {
		return 0, nil, nil, err
	}
	byteJSON, _ := json.Marshal(spec)

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("", "err", err)
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) StartVirtualMachine(node string, spec api.VmSpec) (int, []byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/startVm")
	if err != nil {
		return 0, nil, nil, err
	}
	byteJSON, _ := json.Marshal(spec)

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("", "err", err)
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}
