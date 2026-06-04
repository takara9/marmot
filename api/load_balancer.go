package api

// LoadBalancer defines model for LoadBalancer.
type LoadBalancer struct {
	ApiVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   Metadata         `json:"metadata" yaml:"metadata"`
	Spec       LoadBalancerSpec `json:"spec" yaml:"spec"`
	Status     *Status          `json:"status,omitempty" yaml:"status,omitempty"`
}

// LoadBalancerSpec defines model for LoadBalancerSpec.
type LoadBalancerSpec struct {
	BindPublicIpAddress    string                 `json:"bindPublicIpAddress" yaml:"bindPublicIpAddress"`
	InternalVirtualNetwork string                 `json:"internalVirtualNetwork" yaml:"internalVirtualNetwork"`
	Listeners              []LoadBalancerListener `json:"listeners" yaml:"listeners"`
	RemoteCIDR             string                 `json:"remoteCIDR,omitempty" yaml:"remoteCIDR,omitempty"`
}

// LoadBalancerListener defines one VIP/backend forwarding rule.
type LoadBalancerListener struct {
	BackendPort            int                        `json:"backendPort" yaml:"backendPort"`
	BackendSelector        LoadBalancerLabelSelector  `json:"backendSelector" yaml:"backendSelector"`
	HealthCheck            *LoadBalancerHealthCheck   `json:"healthCheck,omitempty" yaml:"healthCheck,omitempty"`
	LoadBalancingAlgorithm string                     `json:"loadBalancingAlgorithm,omitempty" yaml:"loadBalancingAlgorithm,omitempty"`
	Name                   string                     `json:"name" yaml:"name"`
	Protocol               string                     `json:"protocol" yaml:"protocol"`
	SessionPersistence     *LoadBalancerPersistence   `json:"sessionPersistence,omitempty" yaml:"sessionPersistence,omitempty"`
	VipPort                int                        `json:"vipPort" yaml:"vipPort"`
}

// LoadBalancerLabelSelector defines model for listener backend selection.
type LoadBalancerLabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty" yaml:"matchLabels,omitempty"`
}

// LoadBalancerHealthCheck defines model for HTTP health checks.
type LoadBalancerHealthCheck struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	IntervalSeconds    int    `json:"intervalSeconds,omitempty" yaml:"intervalSeconds,omitempty"`
	Path               string `json:"path,omitempty" yaml:"path,omitempty"`
	TimeoutSeconds     int    `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
	UnhealthyThreshold int    `json:"unhealthyThreshold,omitempty" yaml:"unhealthyThreshold,omitempty"`
}

// LoadBalancerPersistence defines model for listener session persistence.
type LoadBalancerPersistence struct {
	CookieName string `json:"cookieName,omitempty" yaml:"cookieName,omitempty"`
	Enabled    bool   `json:"enabled" yaml:"enabled"`
}