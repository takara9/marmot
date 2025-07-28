package main

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

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
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

func (m *MarmotEndpoint) ListHypervisors() (int, []byte, *url.URL, error) {
	url := m.setUrl("/hypervisors")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Accept", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) ListVirtualMachines() (int, []byte, *url.URL, error) {
	url := m.setUrl("/virtualMachines")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Accept", "application/json")
	return m.httpRequest(req)
}

func (m *MarmotEndpoint) CreateVirtualMachine(vm api.VmSpec) (int, []byte, *url.URL, error) {
	jsonBytes, err := json.Marshal(vm)
	if err != nil {
		fmt.Println("#1")
		return 0, nil, nil, err
	}
	fmt.Println("jsonBytes=", string(jsonBytes))

	req, err := http.NewRequest("POST", m.setUrl("/createVm"), bytes.NewBuffer(jsonBytes))
	if err != nil {
		fmt.Println("#2")
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "MarmotdClient/1.0")
	req.Header.Set("Content-Type", "application/json")
	return m.httpRequest(req)
}

type msg struct {
	Msg string
}

func ReqRest(cnf config.MarmotConfig, apipath string, api string) (*http.Response, []byte, error) {
	byteJSON, _ := json.MarshalIndent(cnf, "", "    ")
	reqURL := fmt.Sprintf("%s/api/v1/%s", api, apipath)
	fmt.Println("request URL = ", reqURL)
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("request by HTTP-POST", "err", err)
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		slog.Error("http client", "err", err)
		return resp, nil, err
	}
	defer resp.Body.Close()

	// レスポンスを取得する
	body, err := io.ReadAll(resp.Body)
	var ErrMsg msg
	fmt.Println(" http Status Code =", resp.StatusCode)

	if resp.StatusCode != 200 {
		fmt.Println("失敗")
		json.Unmarshal(body, &ErrMsg)
		fmt.Println("エラーメッセージ:", ErrMsg.Msg)
	} else {
		fmt.Println("成功終了")
	}
	return resp, body, err
}
