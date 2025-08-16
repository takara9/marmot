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

func (m *MarmotEndpoint) CreateCluster(vmcluster api.MarmotConfig) (int, []byte, *url.URL, error) {
	jsonBytes, err := json.Marshal(vmcluster)
	if err != nil {
		fmt.Println("#1")
		return 0, nil, nil, err
	}
	fmt.Println("jsonBytes=", string(jsonBytes))

	req, err := http.NewRequest("POST", m.setUrl("/createClustercreateVm"), bytes.NewBuffer(jsonBytes))
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


func convertVMSpec(v api.VmSpec) config.VMSpec {
	var x config.VMSpec

	x.Name = *v.Name
	x.CPU = int(*v.Cpu)
	x.Memory = int(*v.Memory)

	if v.PrivateIp != nil {
		x.PrivateIP = *v.PrivateIp
	}

	if v.PublicIp != nil {
		x.PublicIP = *v.PublicIp
	}
	
	if x.Storage != nil {
		fmt.Println("================ *v.Storage =", len(*v.Storage))
		x.Storage = make([]config.Storage, len(*v.Storage))
		for k2, v2 := range *v.Storage {
			x.Storage[k2].Name = *v2.Name
			x.Storage[k2].Size = int(*v2.Size)
			x.Storage[k2].Path = *v2.Path
			x.Storage[k2].VolGrp = *v2.Vg
			x.Storage[k2].Type = *v2.Type
		}
	}

	return x
}

func convertToMarmotConfig(apix api.MarmotConfig) config.MarmotConfig {
	var cnf config.MarmotConfig
	// OpenAPIの構造体から、内部の設定構造体に変換

	if apix.Domain != nil {
		cnf.Domain = *apix.Domain
	} else {
		cnf.Domain = ""
	}

	if apix.Domain != nil {
		cnf.Domain = *apix.Domain
	} else {
		cnf.Domain = ""
	}

	if apix.ClusterName != nil {
		cnf.ClusterName = *apix.ClusterName
	} else {
		cnf.Domain = ""
	}

	if apix.Hypervisor != nil {
		cnf.Hypervisor = *apix.Hypervisor
	} else {
		cnf.Hypervisor = ""
	}

	if apix.ImgaeTemplatePath != nil {
		cnf.VmImageTempPath = *apix.ImgaeTemplatePath
	} else {
		cnf.VmImageTempPath = ""
	}

	if apix.ImgaeTemplatePath != nil {
		cnf.VmImageTempPath = *apix.ImgaeTemplatePath
	} else {
		cnf.VmImageTempPath = ""
	}

	if apix.ImgaeTemplatePath != nil {
		cnf.VMImageDfltPath = *apix.ImgaeTemplatePath
	} else {
		cnf.VMImageDfltPath = ""
	}

	if apix.Qcow2Image != nil {
		cnf.VMImageQCOW = *apix.Qcow2Image
	} else {
		cnf.VMImageQCOW = ""
	}

	cnf.VMOsVariant = *apix.OsVariant
	cnf.NetDevDefault = *apix.NetDevDefault
	cnf.NetDevPrivate = *apix.NetDevPrivate
	cnf.NetDevPublic = *apix.NetDevPublic
	cnf.PublicIPGw = *apix.PublicIpGw
	cnf.PublicIPDns = *apix.PublicIpSubnet
	cnf.PublicIPSubnet = *apix.PublicIpSubnet
	cnf.PrivateIPSubnet = *apix.PrivateIpSubnet
	cnf.VMSpec = make([]config.VMSpec, len(*apix.VmSpec))
	for k, v := range *apix.VmSpec {
		cnf.VMSpec[k].Name = *v.Name
		cnf.VMSpec[k].CPU = int(*v.Cpu)
		cnf.VMSpec[k].Memory = int(*v.Memory)
		cnf.VMSpec[k].PrivateIP = *v.PrivateIp
		cnf.VMSpec[k].PublicIP = *v.PublicIp
		if v.Storage != nil {
			fmt.Println("================ *v.Storage =", len(*v.Storage))
			cnf.VMSpec[k].Storage = make([]config.Storage, len(*v.Storage))
			for k2, v2 := range *v.Storage {
				cnf.VMSpec[k].Storage[k2].Name = *v2.Name
				cnf.VMSpec[k].Storage[k2].Size = int(*v2.Size)
				cnf.VMSpec[k].Storage[k2].Path = *v2.Path
				cnf.VMSpec[k].Storage[k2].VolGrp = *v2.Vg
				cnf.VMSpec[k].Storage[k2].Type = *v2.Type
			}
		}
	}
	return cnf
}

func convertToMarmotConfig2(cnf config.MarmotConfig) api.MarmotConfig {
	var x api.MarmotConfig

	x.Domain = &cnf.Domain
	x.ClusterName = &cnf.ClusterName
	x.Hypervisor = &cnf.Hypervisor
	x.ImageDefaultPath = &cnf.VmImageTempPath
	x.ImgaeTemplatePath = &cnf.VMImageDfltPath
	x.Qcow2Image = &cnf.VMImageQCOW
	x.OsVariant = &cnf.VMOsVariant
	x.PrivateIpSubnet = &cnf.PrivateIPSubnet
	x.PublicIpSubnet = &cnf.PublicIPSubnet
	x.NetDevDefault = &cnf.NetDevDefault
	x.NetDevPrivate = &cnf.NetDevPrivate
	x.NetDevPublic = &cnf.NetDevPublic
	x.PublicIpGw = &cnf.PublicIPGw
	x.PublicIpSubnet = &cnf.PublicIPDns

	var xa []api.VmSpec
	for _, v := range cnf.VMSpec {
		var x1 api.VmSpec
		x1.Name = &v.Name
		cpu := int32(v.CPU)
		x1.Cpu = &cpu
		mem := int64(v.Memory)
		x1.Memory = &mem
		x1.PrivateIp = &v.PrivateIP
		x1.PublicIp = &v.PublicIP
		x1.Playbook = &v.AnsiblePB
		x1.Comment = &v.Comment
		x1.Uuid = &v.Uuid
		x1.Key = &v.Key
		x1.Ostempvg = &v.OsTempVg
		x1.Ostempvariant = &v.VMOsVariant

		var sa []api.Storage
		for _, v2 := range v.Storage {
			var ss api.Storage
			ss.Name = &v2.Name
			size := int64(v2.Size)
			ss.Size = &size
			ss.Path = &v2.Path
			ss.Vg = &v2.VolGrp
			ss.Type = &v2.Type
			sa = append(sa, ss)
		}
		x1.Storage = &sa
		xa = append(xa, x1)
	}
	x.VmSpec = &xa
	return x
}

func printMarmotConf(cnf config.MarmotConfig) {

	// ここで、構造体の変換を実施して、リクエストを送信する
	fmt.Println("Domain=", cnf.Domain)
	fmt.Println("ClusterName=", cnf.ClusterName)

	fmt.Println("Hypervisor=", cnf.Hypervisor)
	fmt.Println("VmImageTempPath=", cnf.VmImageTempPath)
	fmt.Println("VMImageDfltPath=", cnf.VMImageDfltPath)
	fmt.Println("VMImageQCOW=", cnf.VMImageQCOW)
	fmt.Println("VMOsVariant=", cnf.VMOsVariant)
	fmt.Println("PrivateIPSubnet=", cnf.PrivateIPSubnet)
	fmt.Println("PublicIPSubnet=", cnf.PublicIPSubnet)
	fmt.Println("NetDevDefault=", cnf.NetDevDefault)
	fmt.Println("NetDevPrivate=", cnf.NetDevPrivate)
	fmt.Println("NetDevPublic=", cnf.NetDevPublic)
	fmt.Println("PublicIPGw=", cnf.PublicIPGw)
	fmt.Println("PublicIPDns=", cnf.PublicIPDns)
	for i, v := range cnf.VMSpec {
		fmt.Println("VM No=", i)
		fmt.Println("  Name=", v.Name)
		fmt.Println("  CPU=", v.CPU)
		fmt.Println("  Memory=", v.Memory)
		fmt.Println("  PrivateIP=", v.PrivateIP)
		fmt.Println("  PublicIP=", v.PublicIP)
		fmt.Println("  AnsiblePB=", v.AnsiblePB)
		fmt.Println("  Comment=", v.Comment)
		fmt.Println("  Uuid=", v.Uuid)
		fmt.Println("  Key=", v.Key)
		fmt.Println("  OsTempVg=", v.OsTempVg)
		fmt.Println("  VMOsVariant=", v.VMOsVariant)
		for i2, v2 := range v.Storage {
			fmt.Println("Stg No=", i2)
			fmt.Println("    Name=", v2.Name)
			fmt.Println("    Size=", v2.Size)
			fmt.Println("    Path=", v2.Path)
			fmt.Println("    VolGrp=", v2.VolGrp)
			fmt.Println("    Type=", v2.Type)
		}
	}
}
