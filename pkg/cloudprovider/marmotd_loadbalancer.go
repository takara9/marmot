package cloudprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
)

// MarmotdLoadBalancer は marmotd の kubernetes-engine/loadbalancer/vip API を参照して
// LoadBalancer を実装する(フェーズ11の「ロードバランサーコントローラー」のVIP払い出し・解放
// ロジックをmarmot-nativeインターフェースとして移植したもの)。
type MarmotdLoadBalancer struct {
	endpoint *client.MarmotEndpoint
}

// NewMarmotdLoadBalancer は MarmotdLoadBalancer を組み立てる。
func NewMarmotdLoadBalancer(endpoint *client.MarmotEndpoint) *MarmotdLoadBalancer {
	return &MarmotdLoadBalancer{endpoint: endpoint}
}

// EnsureLoadBalancer は marmotd に対してVIP払い出しをリクエストする。
func (lb *MarmotdLoadBalancer) EnsureLoadBalancer(_ context.Context, clusterID string, svc LoadBalancerService) (string, error) {
	reqBody := api.KubernetesEngineLoadBalancerVipRequest{
		Namespace:   svc.Namespace,
		ServiceName: svc.Name,
	}
	body, _, err := lb.endpoint.CreateKubernetesEngineLoadBalancerVip(clusterID, reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to request VIP for %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	var vip api.KubernetesEngineLoadBalancerVip
	if err := json.Unmarshal(body, &vip); err != nil {
		return "", fmt.Errorf("failed to decode VIP response for %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	if strings.TrimSpace(vip.Vip) == "" {
		return "", fmt.Errorf("marmotd returned empty VIP for %s/%s", svc.Namespace, svc.Name)
	}
	return vip.Vip, nil
}

// EnsureLoadBalancerDeleted は marmotd に対してVIP解放をリクエストする。
// marmotd側は未払い出しの場合も200を返すため(冪等)、ここでは特別な分岐は不要。
func (lb *MarmotdLoadBalancer) EnsureLoadBalancerDeleted(_ context.Context, clusterID string, svc LoadBalancerService) error {
	if _, _, err := lb.endpoint.DeleteKubernetesEngineLoadBalancerVip(clusterID, svc.Namespace, svc.Name); err != nil {
		return fmt.Errorf("failed to release VIP for %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	return nil
}
