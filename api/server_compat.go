package api

import "strings"

// NormalizeMMImageAlias preserves legacy behavior for callers that still set
// the deprecated osVariant field instead of mmImage.
func (s *Server) NormalizeMMImageAlias() {
	if s == nil {
		return
	}

	mmImage := ""
	if s.Spec.MmImage != nil {
		mmImage = strings.TrimSpace(*s.Spec.MmImage)
	}
	osVariant := ""
	if s.Spec.OsVariant != nil {
		osVariant = strings.TrimSpace(*s.Spec.OsVariant)
	}

	if mmImage != "" {
		if osVariant == "" {
			v := mmImage
			s.Spec.OsVariant = &v
		}
		return
	}

	if osVariant != "" {
		v := osVariant
		s.Spec.MmImage = &v
	}
}