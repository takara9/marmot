package api

// VolumeID returns the volume identifier stored in metadata.id.
func VolumeID(v Volume) string {
	if v.Metadata == nil || v.Metadata.Id == nil {
		return ""
	}
	return *v.Metadata.Id
}

// SetVolumeID stores the volume identifier into metadata.id.
func SetVolumeID(v *Volume, id string) {
	if v == nil {
		return
	}
	if v.Metadata == nil {
		v.Metadata = &Metadata{}
	}
	v.Metadata.Id = &id
}
