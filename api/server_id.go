package api

// ServerID returns the server identifier stored in metadata.id.
func ServerID(s Server) string {
	if s.Metadata.Id == nil {
		return ""
	}
	return *s.Metadata.Id
}

// SetServerID stores the server identifier into metadata.id.
func SetServerID(s *Server, id string) {
	if s == nil {
		return
	}
	s.Metadata.Id = &id
}
