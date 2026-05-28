package api

// GatewayID returns the gateway identifier stored in metadata.id.
func GatewayID(g Gateway) string {
	return g.Metadata.Id
}

// SetGatewayID stores the gateway identifier into metadata.id.
func SetGatewayID(g *Gateway, id string) {
	if g == nil {
		return
	}
	g.Metadata.Id = id
}
