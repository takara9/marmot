package client

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
)

type MarmotClient struct {
	NodeName string
}

func NewMarmot(nodeName string, etcdUrl string) (*MarmotClient, error) {
	var mc MarmotClient
	mc.NodeName = nodeName
	return &mc, nil
}

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

func (m *MarmotEndpoint) ListHypervisors(params map[string]string) (int, []byte, *url.URL, error) {
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
	slog.Debug("=====", "mactl ListVirtualMachines", "=====")
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

func (m *MarmotEndpoint) CreateCluster(params api.MarmotConfig) (int, []byte, *url.URL, error) {
	slog.Debug("=====", "mactl CreateCluster", "=====")
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		slog.Error("CreateCluster()", "err", err)
		return 0, nil, nil, err
	}
	//PrintMarmotConfig(params)
	slog.Debug("CreateCluster", "host:port", m.HostPort)
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/createCluster")
	if err != nil {
		slog.Error("CreateCluster()", "err", err)
		return 0, nil, nil, err
	}
	slog.Debug("CreateCluster", "url", url)
	slog.Debug("CreateCluster", "body", string(jsonBytes))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		slog.Error("CreateCluster()", "err", err)
		return 0, nil, nil, err
	}

	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) DestroyCluster(params api.MarmotConfig) (int, []byte, *url.URL, error) {
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

func (m *MarmotEndpoint) StopCluster(params api.MarmotConfig) (int, []byte, *url.URL, error) {
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

func (m *MarmotEndpoint) StartCluster(params api.MarmotConfig) (int, []byte, *url.URL, error) {
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

func (m *MarmotEndpoint) CreateVirtualMachine(spec api.VmSpec) (int, []byte, *url.URL, error) {
	reqURL, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/createVm")
	if err != nil {
		return 0, nil, nil, err
	}
	byteJSON, _ := json.Marshal(spec)

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("/createVM", "err", err)
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) DestroyVirtualMachine(spec api.VmSpec) (int, []byte, *url.URL, error) {
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

func (m *MarmotEndpoint) StopVirtualMachine(spec api.VmSpec) (int, []byte, *url.URL, error) {
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

func (m *MarmotEndpoint) StartVirtualMachine(spec api.VmSpec) (int, []byte, *url.URL, error) {
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

func (m *MarmotEndpoint) CreateVolume() (*api.Version, error) {

	

	return nil, nil
}