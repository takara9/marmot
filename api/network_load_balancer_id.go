package api

// NetworkLoadBalancerID returns the network load balancer identifier stored in metadata.id.
func NetworkLoadBalancerID(lb NetworkLoadBalancer) string {
	return lb.Metadata.Id
}

// SetNetworkLoadBalancerID stores the network load balancer identifier into metadata.id.
func SetNetworkLoadBalancerID(lb *NetworkLoadBalancer, id string) {
	if lb == nil {
		return
	}
	lb.Metadata.Id = id
}
