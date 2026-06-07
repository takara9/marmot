package api

// NetworkLoadBalancerID returns the network load balancer identifier stored in metadata.id.
func NetworkLoadBalancerID(nlb NetworkLoadBalancer) string {
	return nlb.Metadata.Id
}

// SetNetworkLoadBalancerID stores the network load balancer identifier into metadata.id.
func SetNetworkLoadBalancerID(nlb *NetworkLoadBalancer, id string) {
	if nlb == nil {
		return
	}
	nlb.Metadata.Id = id
}
