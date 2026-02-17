package marmotd

import (
	_ "embed"

	"github.com/labstack/echo/v4"
)

// Get Network Information
// (GET /network)
func (s *Server) GetNetworks(ctx echo.Context) error {
	return nil
}

// Create Virtual Network
// (POST /network)
func (s *Server) CreateNetwork(ctx echo.Context) error {
	return nil
}

// Delete Virtual Network
// (DELETE /network/{id})
func (s *Server) DeleteNetworkById(ctx echo.Context, id string) error {
	return nil
}

// Get particular virtual network Information
// (GET /network/{id})
func (s *Server) GetNetworkById(ctx echo.Context, id string) error {
	return nil
}

// Update Virtual Network Information by Id
// (PUT /network/{id})
func (s *Server) UpdateNetworkById(ctx echo.Context, id string) error {
	return nil
}
