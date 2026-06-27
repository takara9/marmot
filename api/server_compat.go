package api

import "strings"

// NormalizeMMImageAlias preserves legacy behavior for callers that still set
// the deprecated osVariant field instead of mmImage.
func (s *Server) NormalizeMMImageAlias() {
	if s == nil {
		return
	}

	if s.Spec.MmImage != nil && strings.TrimSpace(*s.Spec.MmImage) != "" {
		return
	}

	if s.Spec.OsVariant != nil {
		v := strings.TrimSpace(*s.Spec.OsVariant)
		if v != "" {
			s.Spec.MmImage = &v
		}
	}
}