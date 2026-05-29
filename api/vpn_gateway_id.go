package api

// VpnGatewayID returns the vpn gateway identifier stored in metadata.id.
func VpnGatewayID(g VpnGateway) string {
	return g.Metadata.Id
}

// SetVpnGatewayID stores the vpn gateway identifier into metadata.id.
func SetVpnGatewayID(g *VpnGateway, id string) {
	if g == nil {
		return
	}
	g.Metadata.Id = id
}
