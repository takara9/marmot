package marmotd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

func (s *Server) ApiCreateVpnGateway(ctx echo.Context) error {
	var rec api.VpnGateway
	if err := ctx.Bind(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	assignNodeNameIfUnset(&rec.Metadata, s.Ma.NodeName)
	if err := normalizeVpnGatewayResource(&rec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	created, err := s.Ma.Db.CreateVpnGateway(rec)
	if err != nil {
		if errors.Is(err, db.ErrFound) {
			return ctx.JSON(http.StatusConflict, api.Error{Code: 1, Message: err.Error()})
		}
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusCreated, created)
}

func (s *Server) ApiGetVpnGateways(ctx echo.Context) error {
	items, err := s.Ma.Db.GetVpnGateways()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, items)
}

func (s *Server) ApiGetVpnGatewayById(ctx echo.Context, id string) error {
	item, err := s.Ma.Db.GetVpnGatewayById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, item)
}

func (s *Server) ApiUpdateVpnGatewayById(ctx echo.Context, id string) error {
	current, err := s.Ma.Db.GetVpnGatewayById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	var req api.VpnGateway
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "invalid request body"})
	}
	if err := normalizeVpnGatewaySpec(&req.Spec); err != nil {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: err.Error()})
	}
	if changedImmutableVpnGatewayField(current, req) {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "immutable fields changed: spec.bindPublicIpAddress, spec.internalVirtualNetwork"})
	}
	current.Spec.RemoteCIDRs = req.Spec.RemoteCIDRs
	if err := s.Ma.Db.UpdateVpnGatewayById(id, current); err != nil {
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to update the vpn gateway")})
}

func (s *Server) ApiDeleteVpnGatewayById(ctx echo.Context, id string) error {
	if _, err := s.Ma.Db.GetVpnGatewayById(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	if err := s.Ma.Db.SetDeleteTimestampVpnGateway(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}
	return ctx.JSON(http.StatusOK, api.Success{Id: id, Message: util.StringPtr("Accepted the request to delete the vpn gateway")})
}

func (s *Server) ApiGetVpnGatewayCertById(ctx echo.Context, id string) error {
	item, err := s.Ma.Db.GetVpnGatewayById(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "IDが存在しません"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	networkName := strings.TrimSpace(item.Spec.InternalVirtualNetwork)
	if networkName == "" {
		return ctx.JSON(http.StatusBadRequest, api.Error{Code: 1, Message: "spec.internalVirtualNetwork is empty"})
	}

	certPath := filepath.Join("/var/lib/marmot/vpn", filepath.Base(networkName)+".ovpn")
	content, err := os.ReadFile(certPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ctx.JSON(http.StatusNotFound, api.Error{Code: 1, Message: "vpn cert is not found"})
		}
		return ctx.JSON(http.StatusInternalServerError, api.Error{Code: 1, Message: err.Error()})
	}

	return ctx.Blob(http.StatusOK, "text/plain; charset=utf-8", content)
}

func changedImmutableVpnGatewayField(current api.VpnGateway, req api.VpnGateway) bool {
	return strings.TrimSpace(current.Spec.BindPublicIpAddress) != strings.TrimSpace(req.Spec.BindPublicIpAddress) ||
		strings.TrimSpace(current.Spec.InternalVirtualNetwork) != strings.TrimSpace(req.Spec.InternalVirtualNetwork)
}

func normalizeVpnGatewayResource(rec *api.VpnGateway) error {
	if rec == nil {
		return fmt.Errorf("vpn gateway is nil")
	}
	if strings.TrimSpace(rec.ApiVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(rec.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(rec.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	return normalizeVpnGatewaySpec(&rec.Spec)
}

func normalizeVpnGatewaySpec(spec *api.VpnGatewaySpec) error {
	if spec == nil {
		return fmt.Errorf("spec is required")
	}
	publicIP := strings.TrimSpace(spec.BindPublicIpAddress)
	if publicIP == "" {
		return fmt.Errorf("spec.bindPublicIpAddress is required")
	}
	if ip := net.ParseIP(publicIP); ip == nil {
		return fmt.Errorf("spec.bindPublicIpAddress must be a valid IP address")
	}
	spec.BindPublicIpAddress = publicIP

	vnet := strings.TrimSpace(spec.InternalVirtualNetwork)
	if vnet == "" {
		return fmt.Errorf("spec.internalVirtualNetwork is required")
	}
	spec.InternalVirtualNetwork = vnet

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(spec.RemoteCIDRs))
	for _, cidr := range spec.RemoteCIDRs {
		trimmed := strings.TrimSpace(cidr)
		if trimmed == "" {
			continue
		}
		ip, _, err := net.ParseCIDR(trimmed)
		if err != nil {
			return fmt.Errorf("spec.remoteCIDRs contains an invalid CIDR: %s", trimmed)
		}
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("spec.remoteCIDRs must contain only IPv4 CIDRs")
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	spec.RemoteCIDRs = normalized
	return nil
}
