package api

// VolumeID returns the volume identifier stored in metadata.id.
func VolumeID(v Volume) string {
	return v.Metadata.Id
}

// SetVolumeID stores the volume identifier into metadata.id.
func SetVolumeID(v *Volume, id string) {
	if v == nil {
		return
	}
	v.Metadata.Id = id
}
