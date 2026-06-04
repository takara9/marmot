package api

// LoadBalancerID returns the load balancer identifier stored in metadata.id.
func LoadBalancerID(lb LoadBalancer) string {
	return lb.Metadata.Id
}

// SetLoadBalancerID stores the load balancer identifier into metadata.id.
func SetLoadBalancerID(lb *LoadBalancer, id string) {
	if lb == nil {
		return
	}
	lb.Metadata.Id = id
}