package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// marmotdClient は marmotd REST API を呼び出し、Node の host-bridge 接続アドレスを収集したり、
// Service type=LoadBalancer 用のVIPを払い出し/解放したりするために必要な最小限のクライアント。
type marmotdClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	keID       string
}

// loadMarmotdClient は、apiKeyPath からmarmotd APIKeyトークンを読み込み、クライアントを組み立てる。
func loadMarmotdClient(baseURL, apiKeyPath, keID string) (*marmotdClient, error) {
	raw, err := os.ReadFile(apiKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read marmotd API key file %q: %w", apiKeyPath, err)
	}
	apiKey := strings.TrimSpace(string(raw))
	if apiKey == "" {
		return nil, fmt.Errorf("marmotd API key file %q is empty", apiKeyPath)
	}
	return &marmotdClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/api/v1",
		apiKey:     apiKey,
		keID:       strings.TrimSpace(keID),
	}, nil
}

// marmotdNetworkInterface は marmotd Server.spec.networkInterface[] の必要フィールドのみ。
type marmotdNetworkInterface struct {
	Networkname string `json:"networkname"`
	Address     string `json:"address"`
}

type marmotdServer struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		NetworkInterface []marmotdNetworkInterface `json:"networkInterface"`
	} `json:"spec"`
}

// hostBridgeAddressesByServerName は、marmotd上の全サーバーから
// host-bridgeネットワークインターフェースのaddressを、サーバー名をキーに収集する。
func (c *marmotdClient) hostBridgeAddressesByServerName(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/server", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marmotd GET /server returned status %d", resp.StatusCode)
	}

	var servers []marmotdServer
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("failed to decode marmotd /server response: %w", err)
	}

	addresses := make(map[string]string, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Metadata.Name)
		if name == "" {
			continue
		}
		for _, nic := range server.Spec.NetworkInterface {
			if nic.Networkname == "host-bridge" && strings.TrimSpace(nic.Address) != "" {
				addresses[name] = strings.TrimSpace(nic.Address)
				break
			}
		}
	}
	return addresses, nil
}

type marmotdVipRequest struct {
	Namespace   string `json:"namespace"`
	ServiceName string `json:"serviceName"`
}

type marmotdVip struct {
	Vip  string `json:"vip"`
	Fqdn string `json:"fqdn"`
}

// requestVip は、marmotdのhost-bridge IPAMプールからService用のVIPを払い出し、内部DNSへ登録する。
// 既に払い出し済みの場合は同じVIPが返る(冪等)。
func (c *marmotdClient) requestVip(ctx context.Context, namespace, serviceName string) (string, error) {
	if c.keID == "" {
		return "", fmt.Errorf("kubernetes engine id is not configured")
	}
	body, err := json.Marshal(marmotdVipRequest{Namespace: namespace, ServiceName: serviceName})
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("/kubernetes-engine/%s/loadbalancer/vip", url.PathEscape(c.keID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("marmotd POST %s returned status %d", path, resp.StatusCode)
	}

	var vip marmotdVip
	if err := json.NewDecoder(resp.Body).Decode(&vip); err != nil {
		return "", fmt.Errorf("failed to decode marmotd vip response: %w", err)
	}
	return vip.Vip, nil
}

// releaseVip は、marmotdへService用VIPの解放(内部DNS削除+IPAM返却)を要求する。
// 既に解放済みの場合も成功として扱われる(冪等)。
func (c *marmotdClient) releaseVip(ctx context.Context, namespace, serviceName string) error {
	if c.keID == "" {
		return fmt.Errorf("kubernetes engine id is not configured")
	}
	path := fmt.Sprintf("/kubernetes-engine/%s/loadbalancer/vip/%s/%s", url.PathEscape(c.keID), url.PathEscape(namespace), url.PathEscape(serviceName))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("marmotd DELETE %s returned status %d", path, resp.StatusCode)
	}
	return nil
}
