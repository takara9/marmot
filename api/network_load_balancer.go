package api

// NetworkLoadBalancer defines model for a future L4 load balancer.
// This type is intentionally separated from ApplicationLoadBalancer (L7).
type NetworkLoadBalancer struct {
	ApiVersion string                  `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                  `json:"kind" yaml:"kind"`
	Metadata   Metadata                `json:"metadata" yaml:"metadata"`
	Spec       NetworkLoadBalancerSpec `json:"spec" yaml:"spec"`
	Status     *Status                 `json:"status,omitempty" yaml:"status,omitempty"`
}

type NetworkLoadBalancerSpec struct {
	BindPublicIpAddress    string                        `json:"bindPublicIpAddress" yaml:"bindPublicIpAddress"`
	InternalVirtualNetwork string                        `json:"internalVirtualNetwork" yaml:"internalVirtualNetwork"`
	Listeners              []NetworkLoadBalancerListener `json:"listeners" yaml:"listeners"`
	RemoteCIDR             string                        `json:"remoteCIDR,omitempty" yaml:"remoteCIDR,omitempty"`
}

type NetworkLoadBalancerListener struct {
	BackendPort        int                              `json:"backendPort" yaml:"backendPort"`
	BackendSelector    NetworkLoadBalancerLabelSelector `json:"backendSelector" yaml:"backendSelector"`
	Name               string                           `json:"name" yaml:"name"`
	Protocol           string                           `json:"protocol" yaml:"protocol"`
	SessionPersistence *NetworkLoadBalancerPersistence  `json:"sessionPersistence,omitempty" yaml:"sessionPersistence,omitempty"`
	VipPort            int                              `json:"vipPort" yaml:"vipPort"`
}

type NetworkLoadBalancerLabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty" yaml:"matchLabels,omitempty"`
}

type NetworkLoadBalancerPersistence struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}
