// Package cloudprovider は、KubernetesEngine (MKE) 向け cloud-controller-manager が
// 依存する予定の cloud-provider インターフェースの marmot-native な先行実装を提供する。
// k8s.io/cloud-provider への依存追加はまだ行わず、Instances/InstancesV2、LoadBalancer相当の
// 最小インターフェースと実装のみをここに置く(Region/Zoneの提供はInstanceMetadataで対応)。
// Routes は対象外。
package cloudprovider

import "context"

// NodeAddressType は NodeAddress.Type に設定するアドレス種別。
type NodeAddressType string

const (
	NodeInternalIP NodeAddressType = "InternalIP"
	NodeExternalIP NodeAddressType = "ExternalIP"
	NodeHostName   NodeAddressType = "Hostname"
)

// NodeAddress は cloud-provider の v1.NodeAddress に相当する最小表現。
type NodeAddress struct {
	Type    NodeAddressType
	Address string
}

// InstanceMetadata は cloud-provider の InstanceMetadata に相当する最小表現。
type InstanceMetadata struct {
	ProviderID    string
	InstanceType  string
	NodeAddresses []NodeAddress
	Zone          string
	Region        string
}

// Instances は cloud-provider の Instances/InstancesV2 相当の marmot-native インターフェース。
// nodeName は Kubernetes Node の名前(= marmotd Server の metadata.name)を指す。
type Instances interface {
	// InstanceExists は、指定ノードに対応する仮想サーバーが marmotd 上に存在するかを返す。
	InstanceExists(ctx context.Context, nodeName string) (bool, error)
	// InstanceShutdown は、指定ノードに対応する仮想サーバーが停止状態かを返す。
	InstanceShutdown(ctx context.Context, nodeName string) (bool, error)
	// InstanceMetadata は、指定ノードのProviderID/アドレス等のメタデータを返す。
	InstanceMetadata(ctx context.Context, nodeName string) (*InstanceMetadata, error)
}

// LoadBalancerService は cloud-provider の v1.Service(type=LoadBalancer)に相当する最小表現。
type LoadBalancerService struct {
	Namespace string
	Name      string
}

// LoadBalancer は cloud-provider の LoadBalancer 相当の marmot-native インターフェース。
// フェーズ11の「ロードバランサーコントローラー」が行っていたVIP払い出し・内部DNS登録・解放を、
// marmotd の kubernetes-engine/loadbalancer/vip API 経由で実施する。
type LoadBalancer interface {
	// EnsureLoadBalancer は、svc に対応するVIPが未払い出しなら払い出し、内部DNSへ登録した上で
	// VIPを返す。既に払い出し済みの場合は同じVIPを返す(冪等)。
	EnsureLoadBalancer(ctx context.Context, clusterID string, svc LoadBalancerService) (vip string, err error)
	// EnsureLoadBalancerDeleted は、svc に払い出し済みのVIPと内部DNSエントリーを解放する。
	// 未払い出しの場合も成功として扱う(冪等)。
	EnsureLoadBalancerDeleted(ctx context.Context, clusterID string, svc LoadBalancerService) error
}
