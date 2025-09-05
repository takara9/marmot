package marmot

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
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
	req.Header.Set("Accept", "application/json")

	// パラメーターが無いケースもある？
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
	req.Header.Set("Accept", "application/json")

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
	req.Header.Set("Accept", "application/json")

	// パラメーターが無いケースもある？
	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	return m.httpRequest(req)
}

// Additional methods for other API endpoints can be added here following the same pattern.			

func (m *MarmotEndpoint) CreateCluster(params map[string]string) (int, []byte, *url.URL, error) {
	fmt.Println("Client: ListVirtualMachines")
	url, err := url.JoinPath(m.Scheme+"://"+m.HostPort, m.BasePath, "/createCluster")
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Accept", "application/json")



	return m.httpRequest(req)
}
