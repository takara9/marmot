package api

// LoadBalancerID returns the load balancer identifier stored in metadata.id.
func LoadBalancerID(lb ApplicationLoadBalancer) string {
	return lb.Metadata.Id
}

// SetLoadBalancerID stores the load balancer identifier into metadata.id.
func SetLoadBalancerID(lb *ApplicationLoadBalancer, id string) {
	if lb == nil {
		return
	}
	lb.Metadata.Id = id
}