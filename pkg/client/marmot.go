package client

import (
	"crypto/tls"
	"net/http"
	"time"
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
