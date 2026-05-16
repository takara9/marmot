package api

// ServerID returns the server identifier stored in metadata.id.
func ServerID(s Server) string {
	return s.Metadata.Id
}

// SetServerID stores the server identifier into metadata.id.
func SetServerID(s *Server, id string) {
	if s == nil {
		return
	}
	s.Metadata.Id = id
}
