package api

// VirtualNetworkID returns the virtual network identifier stored in metadata.id.
func VirtualNetworkID(v VirtualNetwork) string {
	if v.Metadata.Id == nil {
		return ""
	}
	return *v.Metadata.Id
}

// SetVirtualNetworkID stores the virtual network identifier into metadata.id.
func SetVirtualNetworkID(v *VirtualNetwork, id string) {
	if v == nil {
		return
	}
	v.Metadata.Id = &id
}