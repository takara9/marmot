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

// 新APIから旧APIの構造体へ変換する
func PrintMarmotConfig(a api.MarmotConfig) {
	fmt.Println("=========================================")
	if a.ClusterName != nil {
		fmt.Println("a.ClusterName=", *a.ClusterName)
	}
	if a.Domain != nil {
		fmt.Println("a.Domain=", *a.Domain)
	}
	if a.Hypervisor != nil {
		fmt.Println("a.Hypervisor=", *a.Hypervisor)
	}
	if a.ImageDefaultPath != nil {
		fmt.Println("a.ImageDefaultPath=", *a.ImageDefaultPath)
	}
	if a.ImgaeTemplatePath != nil {
		fmt.Println("a.ImgaeTemplatePath=", *a.ImgaeTemplatePath)
	}
	if a.NetDevDefault != nil {
		fmt.Println("a.NetDevDefault=", *a.NetDevDefault)
	}
	if a.NetDevPrivate != nil {
		fmt.Println("a.NetDevPrivate=", *a.NetDevPrivate)
	}
	if a.OsVariant != nil {
		fmt.Println("a.OsVariant=", *a.OsVariant)
	}
	if a.PrivateIpSubnet != nil {
		fmt.Println("a.PrivateIpSubnet=", *a.PrivateIpSubnet)
	}
	if a.PublicIpDns != nil {
		fmt.Println("a.PublicIpDns=", *a.PublicIpDns)
	}
	if a.PublicIpGw != nil {
		fmt.Println("a.PublicIpGw=", *a.PublicIpGw)
	}
	if a.PublicIpSubnet != nil {
		fmt.Println("a.PublicIpSubnet=", *a.PublicIpSubnet)
	}
	if a.Qcow2Image != nil {
		fmt.Println("a.Qcow2Image=", *a.Qcow2Image)
	}

	// ここでエラーになるのは、クライアントライブラリが未完のため
	if a.VmSpec != nil {
		for _, v := range *a.VmSpec {

			if v.Name != nil {
				fmt.Println("v.Name=", *v.Name)
			}
			if v.Cpu != nil {
				fmt.Println("v.Cpu=", int(*v.Cpu))
			}
			if v.Memory != nil {
				fmt.Println("v.Memory=", *v.Memory)
			}
			if v.PrivateIp != nil {
				fmt.Println("v.PrivateIp=", *v.PrivateIp)
			}
			if v.PublicIp != nil {
				fmt.Println("v.PublicIp=", *v.PublicIp)
			}
			if v.Comment != nil {
				fmt.Println("v.Comment=", *v.Comment)
			}
			if v.Key != nil {
				fmt.Println("v.Key=", *v.Key)
			}
			if v.Ostemplv != nil {
				fmt.Println("v.Ostemplv=", *v.Ostemplv)
			}
			if v.Ostempvg != nil {
				fmt.Println("v.Ostempvg=", *v.Ostempvg)
			}
			if v.Ostempvariant != nil {
				fmt.Println("v.Ostempvariant=", *v.Ostempvariant)
			}
			if v.Uuid != nil {
				fmt.Println("v.Uuid=", *v.Uuid)
			}
			if v.Playbook != nil {
				fmt.Println("v.Playbook=", *v.Playbook)
			}
			if v.Storage != nil {
				for _, v2 := range *v.Storage {
					if v2.Name != nil {
						fmt.Println("v2.Name=", *v2.Name)
					}
					if v2.Path != nil {
						fmt.Println("v2.Path=", *v2.Path)
					}
					if v2.Size != nil {
						fmt.Println("v2.Size=", int(*v2.Size))
					}
					if v2.Type != nil {
						fmt.Println("v2.Type=", *v2.Type)
					}
					if v2.Vg != nil {
						fmt.Println("v2.Vg=", *v2.Vg)
					}
				}
			}
		}
	}
	fmt.Println("=========================================")
	return
}
